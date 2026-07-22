package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/agentlaunch"
	"github.com/jonnyom/slis/internal/agentreview"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/review"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// reviewStore opens the pending-review-comment store at its state-dir path.
func reviewStore() *review.Store {
	return review.Open(config.StatePaths().Reviews)
}

// memberBranch resolves the branch a slice's repo is on, so a stored comment
// carries the branch it targets. It errors when the slice is unknown or the repo
// is not one of its members — catching a typo at add time rather than at send.
func memberBranch(sl model.Slice, repo string) (string, error) {
	m, ok := sl.Members[repo]
	if !ok {
		return "", fmt.Errorf("repo %q is not a member of slice %q (members: %v)", repo, sl.Name, sl.Repos())
	}
	return m.Branch, nil
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Accumulate inline review comments on a slice and deliver them to its agent",
	Long: "Comment on a slice's diff (file:line + instruction); comments accumulate\n" +
		"into a pending batch. `slis review send <slice>` composes them into one\n" +
		"prompt, starts the configured agent if needed, and injects it into that\n" +
		"agent's active tmux pane so it can\n" +
		"address the feedback. Mutation lives here in the CLI; the read-only RPC\n" +
		"sidecar only lists pending comments.",
}

var reviewAddCmd = &cobra.Command{
	Use:   "add <slice> --repo R --file F --line N [--end-line N] [--side new|old] --body B [--hunk H]",
	Short: "Add a pending review comment on a slice's file:line",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validateSliceName(name); err != nil {
			return err
		}
		repo, _ := cmd.Flags().GetString("repo")
		file, _ := cmd.Flags().GetString("file")
		line, _ := cmd.Flags().GetInt("line")
		endLine, _ := cmd.Flags().GetInt("end-line")
		side, _ := cmd.Flags().GetString("side")
		body, _ := cmd.Flags().GetString("body")
		hunk, _ := cmd.Flags().GetString("hunk")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sl, err := findSlice(ws, name)
		if err != nil {
			return err
		}
		branch, err := memberBranch(sl, repo)
		if err != nil {
			return err
		}
		if endLine > 0 && endLine < line {
			return fmt.Errorf("--end-line must be greater than or equal to --line")
		}
		if side != "new" && side != "old" {
			return fmt.Errorf("--side must be new or old")
		}

		c, err := reviewStore().Add(review.Comment{
			Slice:   name,
			Repo:    repo,
			Branch:  branch,
			File:    file,
			Line:    line,
			EndLine: endLine,
			Side:    side,
			Hunk:    hunk,
			Body:    body,
		})
		if err != nil {
			return err
		}
		location := fmt.Sprintf("%s:%d", c.File, c.Line)
		if c.EndLine > c.Line {
			location = fmt.Sprintf("%s-%d", location, c.EndLine)
		}
		fmt.Printf("added review comment %s on %s %s\n", c.ID, c.Repo, location)
		return nil
	},
}

// renderReviewTable writes an aligned ID/SLICE/LOCATION/BODY table to w.
func renderReviewTable(w io.Writer, comments []review.Comment) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSLICE\tLOCATION\tBODY")
	for _, c := range comments {
		location := fmt.Sprintf("%s:%d", c.File, c.Line)
		if c.EndLine > c.Line {
			location = fmt.Sprintf("%s-%d", location, c.EndLine)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s %s\t%s\n", c.ID, c.Slice, c.Repo, location, c.Body)
	}
	tw.Flush()
}

var reviewListCmd = &cobra.Command{
	Use:   "list [slice]",
	Short: "List pending review comments (all slices, or one)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")
		store := reviewStore()

		var (
			comments []review.Comment
			err      error
		)
		if len(args) == 1 {
			comments, err = store.List(args[0])
		} else {
			comments, err = store.ListAll()
		}
		if err != nil {
			return err
		}
		if comments == nil {
			comments = []review.Comment{}
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(comments)
		}
		renderReviewTable(os.Stdout, comments)
		return nil
	},
}

var reviewRmCmd = &cobra.Command{
	Use:   "rm <slice> <id>",
	Short: "Remove a pending review comment by id",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, id := args[0], args[1]
		store := reviewStore()

		// Guard: the id must belong to the named slice, so `rm feat <id-from-other>`
		// can't silently delete a different slice's comment.
		comments, err := store.List(name)
		if err != nil {
			return err
		}
		found := false
		for _, c := range comments {
			if c.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no pending review comment %q on slice %q", id, name)
		}
		if err := store.Remove(id); err != nil {
			return err
		}
		fmt.Printf("removed review comment %s\n", id)
		return nil
	},
}

// runReviewSend delivers a slice's pending comments through sess, clearing them
// on success unless keep is set. It is separated from the cobra wiring so it can
// be tested with a fake session. Returns the number of comments delivered.
func runReviewSend(store *review.Store, slice string, sess review.Session, keep bool) (int, error) {
	comments, err := store.List(slice)
	if err != nil {
		return 0, err
	}
	if len(comments) == 0 {
		return 0, fmt.Errorf("no pending review comments for slice %q", slice)
	}
	if err := review.Send(slice, comments, sess); err != nil {
		if errors.Is(err, review.ErrNoSession) {
			return 0, fmt.Errorf("slice %q session disappeared before delivery; review comments remain pending — re-run `slis review send %s`", slice, slice)
		}
		if errors.Is(err, review.ErrNoAgent) {
			return 0, fmt.Errorf("slice %q has a tmux session, but no agent is running in its active pane; review comments remain pending", slice)
		}
		if errors.Is(err, review.ErrAgentNotReady) {
			return 0, fmt.Errorf("%v; review comments remain pending — press `a`, finish the agent setup, then send again", err)
		}
		return 0, err
	}
	if !keep {
		if err := store.Clear(slice); err != nil {
			return 0, fmt.Errorf("comments delivered but could not clear the pending batch: %w", err)
		}
	}
	return len(comments), nil
}

func storeAgentFindings(store *review.Store, slice model.Slice, author string, findings []agentreview.Finding) ([]review.Comment, error) {
	comments := make([]review.Comment, 0, len(findings))
	for _, finding := range findings {
		member := slice.Members[finding.Repo]
		comment := review.Comment{
			Slice: slice.Name, Repo: finding.Repo, Branch: member.Branch, File: finding.File,
			Line: finding.Line, EndLine: finding.EndLine, Side: finding.Side, Body: finding.Body, Author: author,
		}
		identity, err := json.Marshal(comment)
		if err != nil {
			return comments, err
		}
		comment.ID = fmt.Sprintf("agent-%x", sha256.Sum256(identity))
		comment, err = store.Add(comment)
		if err != nil {
			return comments, err
		}
		comments = append(comments, comment)
	}
	return comments, nil
}

func reviewAgentCommands(s config.Sessions) []string {
	specs := s.AgentList()
	out := make([]string, 0, len(specs))
	seen := make(map[string]bool)
	for _, spec := range specs {
		if len(spec.Cmd) > 0 {
			command := strings.Join(spec.Cmd, " ")
			out = append(out, command)
			seen[command] = true
		}
	}
	for _, command := range []string{"claude", "codex", "opencode", "gemini", "cursor-agent"} {
		if !seen[command] {
			out = append(out, command)
		}
	}
	return out
}

func defaultReviewAgent(s config.Sessions) (command, harness string) {
	if len(s.Agents) > 0 && len(s.Agents[0].Cmd) > 0 {
		return strings.Join(s.Agents[0].Cmd, " "), s.Agents[0].Name
	}
	return s.AgentCommand(), s.HarnessName()
}

func reviewSessionMembers(sl model.Slice) []model.SliceMember {
	repos := sl.Repos()
	members := make([]model.SliceMember, 0, len(repos))
	for _, repo := range repos {
		members = append(members, sl.Members[repo])
	}
	return members
}

func reviewAgentForegroundCommand(executable, slice, agent string) string {
	return strings.Join([]string{
		agentlaunch.ShellSingleQuote(executable),
		"review agent",
		agentlaunch.ShellSingleQuote(slice),
		"--agent",
		agentlaunch.ShellSingleQuote(agent),
		"--foreground",
	}, " ")
}

func launchReviewAgent(ws config.Workspace, sl model.Slice, agent config.AgentSpec) error {
	if !tmuxctl.Available() {
		return errors.New("tmux is required to run a review agent")
	}
	if err := tmuxctl.EnsureSession(sl.Name, reviewSessionMembers(sl), tmuxctl.SessionOpts{
		Root: ws.Root, Layout: ws.Sessions.Layout,
	}); err != nil {
		return fmt.Errorf("ensure review session: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve slis executable: %w", err)
	}
	command := reviewAgentForegroundCommand(executable, sl.Name, agent.Name)
	command += `; status=$?; if [ "$status" -ne 0 ]; then printf '\nReview failed. Press Enter to close.\n'; read -r _; fi; exit "$status"`
	window := "review-" + sanitiseWindowName(agent.Name)
	if err := tmuxctl.StartWindow(sl.Name, window, reviewAgentCwd(sl, ws.Root), command); err != nil {
		return err
	}
	fmt.Printf("%s review started in tmux window %q for slice %q\n", agent.Name, window, sl.Name)
	return nil
}

func sanitiseWindowName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.NewReplacer(" ", "-", ":", "-", ".", "-").Replace(name)
	return name
}

// reviewAgentCwd chooses a cross-repo-safe cwd for a dedicated agent window.
// Created slices share a worktree parent; adopted slices may not, so those fall
// back to the workspace root and rely on the injected worktree context.
func reviewAgentCwd(sl model.Slice, wsRoot string) string {
	repos := sl.Repos()
	if len(repos) == 0 {
		return wsRoot
	}
	first := sl.Members[repos[0]].WorktreePath
	if len(repos) == 1 {
		return first
	}
	parent := filepath.Dir(first)
	for _, repo := range repos[1:] {
		if filepath.Dir(sl.Members[repo].WorktreePath) != parent {
			return wsRoot
		}
	}
	return parent
}

func waitForReviewShell(slice string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		command := tmuxctl.ActivePaneCommand(slice)
		if tmuxctl.IsShellCommand(command) {
			return command
		}
		time.Sleep(50 * time.Millisecond)
	}
	return tmuxctl.ActivePaneCommand(slice)
}

// ensureReviewAgent creates/reuses the slice session and starts the configured
// agent when its active pane is a shell. A busy non-agent pane gets a dedicated
// `agent` window so review delivery never overwrites another process.
func ensureReviewAgent(ws config.Workspace, sl model.Slice, sess review.TmuxSession) error {
	existed := tmuxctl.SessionExists(sl.Name)
	if err := tmuxctl.EnsureSession(sl.Name, reviewSessionMembers(sl), tmuxctl.SessionOpts{
		Root: ws.Root, Layout: ws.Sessions.Layout,
	}); err != nil {
		return err
	}
	if sess.HasAgent(sl.Name) || sess.ActivateAgent(sl.Name) {
		return nil
	}
	command := tmuxctl.ActivePaneCommand(sl.Name)
	if !existed {
		command = waitForReviewShell(sl.Name, 2*time.Second)
	}

	if !tmuxctl.IsShellCommand(command) {
		if err := tmuxctl.SelectOrCreateWindow(sl.Name, "agent", reviewAgentCwd(sl, ws.Root)); err != nil {
			return err
		}
		if sess.HasAgent(sl.Name) {
			return nil
		}
		command = waitForReviewShell(sl.Name, 2*time.Second)
	}
	if !tmuxctl.IsShellCommand(command) {
		return fmt.Errorf("slice %q agent window is busy with %q; review comments remain pending",
			sl.Name, command)
	}

	agent, harness := defaultReviewAgent(ws.Sessions)
	if err := tmuxctl.SendKeys(sl.Name, agentlaunch.Line(agent, sl, ws.Root, harness)); err != nil {
		return fmt.Errorf("launch review agent: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sess.HasAgent(sl.Name) {
			// Give the agent's interactive input loop a moment to initialise after
			// its process first appears in the pane tree.
			time.Sleep(time.Second)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("configured agent %q did not start in slice %q; review comments remain pending", agent, sl.Name)
}

var reviewSendCmd = &cobra.Command{
	Use:   "send <slice>",
	Short: "Compose the pending comments into a prompt and inject it into the slice's session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validateSliceName(name); err != nil {
			return err
		}
		keep, _ := cmd.Flags().GetBool("keep")
		if !tmuxctl.Available() {
			return fmt.Errorf("tmux not found on PATH — `slis review send` delivers through the slice's tmux session")
		}
		store := reviewStore()
		pending, err := store.List(name)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return fmt.Errorf("no pending review comments for slice %q", name)
		}
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sl, err := findSlice(ws, name)
		if err != nil {
			return err
		}
		sess := review.TmuxSession{AgentCommands: reviewAgentCommands(ws.Sessions)}
		if err := ensureReviewAgent(ws, sl, sess); err != nil {
			return err
		}
		n, err := runReviewSend(store, name, sess, keep)
		if err != nil {
			return err
		}
		suffix := ""
		if keep {
			suffix = " (kept pending)"
		}
		fmt.Printf("delivered %d review comment(s) to slice %q%s\n", n, name, suffix)
		return nil
	},
}

var reviewAgentCmd = &cobra.Command{
	Use:   "agent <slice>",
	Short: "Ask a selected agent to review the stack and deliver its findings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validateSliceName(name); err != nil {
			return err
		}
		agentName, _ := cmd.Flags().GetString("agent")
		if agentName == "" {
			return fmt.Errorf("--agent is required")
		}
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		slice, err := findSlice(ws, name)
		if err != nil {
			return err
		}
		agent, err := agentreview.ResolveAgent(ws.Sessions, agentName, exec.LookPath)
		if err != nil {
			return err
		}
		foreground, _ := cmd.Flags().GetBool("foreground")
		if !foreground {
			return launchReviewAgent(ws, slice, agent)
		}
		reviewContext, cancel := context.WithTimeout(cmd.Context(), 15*time.Minute)
		defer cancel()
		findings, err := agentreview.Run(reviewContext, ws.Root, slice, agent, agentreview.ExecuteCommand)
		if err != nil {
			return err
		}
		if len(findings) == 0 {
			fmt.Printf("%s found no review findings on slice %q\n", agent.Name, name)
			return nil
		}
		store := reviewStore()
		comments, err := storeAgentFindings(store, slice, agent.Name, findings)
		if err != nil {
			return err
		}
		if !tmuxctl.Available() {
			return fmt.Errorf("%s stored %d finding(s), but tmux is unavailable so they could not be delivered", agent.Name, len(comments))
		}
		sess := review.TmuxSession{AgentCommands: reviewAgentCommands(ws.Sessions)}
		if err := ensureReviewAgent(ws, slice, sess); err != nil {
			return fmt.Errorf("%s stored %d finding(s), but delivery failed: %w", agent.Name, len(comments), err)
		}
		if err := review.Send(name, comments, sess); err != nil {
			return fmt.Errorf("%s stored %d finding(s), but delivery failed: %w", agent.Name, len(comments), err)
		}
		fmt.Printf("%s stored and delivered %d review finding(s) on slice %q\n", agent.Name, len(comments), name)
		return nil
	},
}

var reviewClearCmd = &cobra.Command{
	Use:   "clear <slice>",
	Short: "Discard all pending review comments for a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		store := reviewStore()
		comments, err := store.List(name)
		if err != nil {
			return err
		}
		if err := store.Clear(name); err != nil {
			return err
		}
		fmt.Printf("cleared %d pending review comment(s) for slice %q\n", len(comments), name)
		return nil
	},
}

func init() {
	reviewAddCmd.Flags().String("repo", "", "Repo the comment targets (required)")
	reviewAddCmd.Flags().String("file", "", "File the comment targets (required)")
	reviewAddCmd.Flags().Int("line", 0, "Line number on the selected diff side (required)")
	reviewAddCmd.Flags().Int("end-line", 0, "Optional end line for a multi-line comment")
	reviewAddCmd.Flags().String("side", "new", "Diff side the line number belongs to: new or old")
	reviewAddCmd.Flags().String("body", "", "The review instruction (required)")
	reviewAddCmd.Flags().String("hunk", "", "Optional diff-hunk excerpt for context")
	_ = reviewAddCmd.MarkFlagRequired("repo")
	_ = reviewAddCmd.MarkFlagRequired("file")
	_ = reviewAddCmd.MarkFlagRequired("line")
	_ = reviewAddCmd.MarkFlagRequired("body")

	reviewListCmd.Flags().Bool("json", false, "Output as JSON")
	reviewSendCmd.Flags().Bool("keep", false, "Keep the pending comments after a successful send")
	reviewAgentCmd.Flags().String("agent", "", "Reviewer agent name")
	reviewAgentCmd.Flags().Bool("foreground", false, "Run the review in the current process")
	_ = reviewAgentCmd.Flags().MarkHidden("foreground")

	reviewCmd.AddCommand(reviewAddCmd)
	reviewCmd.AddCommand(reviewListCmd)
	reviewCmd.AddCommand(reviewRmCmd)
	reviewCmd.AddCommand(reviewSendCmd)
	reviewCmd.AddCommand(reviewAgentCmd)
	reviewCmd.AddCommand(reviewClearCmd)
	rootCmd.AddCommand(reviewCmd)
}

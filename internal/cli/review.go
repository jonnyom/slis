package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

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
		"prompt and injects it into the slice's running session, so the agent can\n" +
		"address the feedback. Mutation lives here in the CLI; the read-only RPC\n" +
		"sidecar only lists pending comments.",
}

var reviewAddCmd = &cobra.Command{
	Use:   "add <slice> --repo R --file F --line N --body B [--hunk H]",
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

		c, err := reviewStore().Add(review.Comment{
			Slice:  name,
			Repo:   repo,
			Branch: branch,
			File:   file,
			Line:   line,
			Hunk:   hunk,
			Body:   body,
		})
		if err != nil {
			return err
		}
		fmt.Printf("added review comment %s on %s %s:%d\n", c.ID, c.Repo, c.File, c.Line)
		return nil
	},
}

// renderReviewTable writes an aligned ID/SLICE/LOCATION/BODY table to w.
func renderReviewTable(w io.Writer, comments []review.Comment) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSLICE\tLOCATION\tBODY")
	for _, c := range comments {
		fmt.Fprintf(tw, "%s\t%s\t%s %s:%d\t%s\n", c.ID, c.Slice, c.Repo, c.File, c.Line, c.Body)
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
			return 0, fmt.Errorf("slice %q has no running session to deliver to — start one (e.g. `slis focus %s`, then launch your agent) and re-run `slis review send %s`", slice, slice, slice)
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
		n, err := runReviewSend(reviewStore(), name, review.TmuxSession{}, keep)
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
	reviewAddCmd.Flags().Int("line", 0, "Line number in the new file (required)")
	reviewAddCmd.Flags().String("body", "", "The review instruction (required)")
	reviewAddCmd.Flags().String("hunk", "", "Optional diff-hunk excerpt for context")
	_ = reviewAddCmd.MarkFlagRequired("repo")
	_ = reviewAddCmd.MarkFlagRequired("file")
	_ = reviewAddCmd.MarkFlagRequired("line")
	_ = reviewAddCmd.MarkFlagRequired("body")

	reviewListCmd.Flags().Bool("json", false, "Output as JSON")
	reviewSendCmd.Flags().Bool("keep", false, "Keep the pending comments after a successful send")

	reviewCmd.AddCommand(reviewAddCmd)
	reviewCmd.AddCommand(reviewListCmd)
	reviewCmd.AddCommand(reviewRmCmd)
	reviewCmd.AddCommand(reviewSendCmd)
	reviewCmd.AddCommand(reviewClearCmd)
	rootCmd.AddCommand(reviewCmd)
}

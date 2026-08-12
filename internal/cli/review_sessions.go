package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/agentlaunch"
	"github.com/jonnyom/slis/internal/agentreview"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/review"
	"github.com/jonnyom/slis/internal/reviewrun"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

func reviewRunStore() *reviewrun.Store {
	return reviewrun.Open(config.StatePaths().StateDir)
}

func persistReviewFailure(store *reviewrun.Store, runID string, reviewErr error) error {
	if statusErr := store.SetFailed(runID, reviewErr.Error()); statusErr != nil {
		return fmt.Errorf("%w; persist review failure: %v", reviewErr, statusErr)
	}
	return reviewErr
}

func reviewRunWindow(agentName, runID string) string {
	name := sanitiseWindowName(agentName)
	if len(name) > 24 {
		name = name[:24]
	}
	suffix := runID
	if len(suffix) > 6 {
		suffix = suffix[len(suffix)-6:]
	}
	return "review-" + name + "-" + suffix
}

func reviewRunCommand(executable, slice, agent, runID string) string {
	return strings.Join([]string{
		agentlaunch.ShellSingleQuote(executable),
		"review agent",
		agentlaunch.ShellSingleQuote(slice),
		"--agent",
		agentlaunch.ShellSingleQuote(agent),
		"--foreground",
		"--run-id",
		agentlaunch.ShellSingleQuote(runID),
	}, " ")
}

func startReviewRunWindow(ws config.Workspace, slice model.Slice, run reviewrun.Run) error {
	manager, err := openSessionManager()
	if err != nil {
		return err
	}
	if _, err := manager.Ensure(context.Background(), slice.Name, reviewSessionMembers(slice), sessionmanager.LayoutOptions{Root: ws.Root, Layout: ws.Sessions.Layout}); err != nil {
		return fmt.Errorf("ensure Slis review session: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve slis executable: %w", err)
	}
	command := reviewRunCommand(executable, slice.Name, run.Agent, run.ID)
	return manager.StartCommand(context.Background(), slice.Name, sessionmanager.TabSpec{ID: run.Window, Kind: sessionmanager.TabKindReview, Title: run.Agent + " review", CWD: reviewAgentCwd(slice, ws.Root)}, command)
}

func pendingReviewMessages(detail reviewrun.Detail) []reviewrun.Message {
	start := 0
	if detail.LastRequest != "" {
		for index, message := range detail.Messages {
			if message.ID == detail.LastRequest {
				start = index + 1
				break
			}
		}
	}
	pending := []reviewrun.Message{}
	for _, message := range detail.Messages[start:] {
		if message.Role == reviewrun.RoleUser {
			pending = append(pending, message)
		}
	}
	return pending
}

func reviewConversationPrompt(slice model.Slice, detail reviewrun.Detail) string {
	var builder strings.Builder
	builder.WriteString(agentreview.Prompt(slice))
	builder.WriteString("\nThe summary field is your complete readable reply to the user. Consider the full conversation below and respond to the latest user message. Preserve valid earlier findings and return only newly discovered findings in comments.\n\n")
	for _, message := range detail.Messages {
		fmt.Fprintf(&builder, "%s:\n%s\n\n", message.Role, message.Body)
	}
	return builder.String()
}

func deliverReviewFindings(ws config.Workspace, slice model.Slice, agentName, deliveryID string, findings []agentreview.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	comments, err := storeAgentFindings(reviewStore(), slice, agentName, findings)
	if err != nil {
		return err
	}
	manager, err := openSessionManager()
	if err != nil {
		return fmt.Errorf("%s stored %d finding(s), but delivery failed: %w", agentName, len(comments), err)
	}
	session := review.SlisSession{Manager: manager, AgentCommands: reviewAgentCommands(ws.Sessions), TabID: "agent", Context: context.Background()}
	if err := ensureReviewAgent(ws, slice, session); err != nil {
		return fmt.Errorf("%s stored %d finding(s), but delivery failed: %w", agentName, len(comments), err)
	}
	if err := session.SendPromptOnce(slice.Name, review.ComposePrompt(comments), deliveryID); err != nil {
		return fmt.Errorf("%s stored %d finding(s), but delivery failed: %w", agentName, len(comments), err)
	}
	return nil
}

func runReviewConversation(ctx context.Context, ws config.Workspace, slice model.Slice, agent config.AgentSpec, runID string) error {
	store := reviewRunStore()
	for {
		detail, err := store.Get(runID)
		if err != nil {
			return err
		}
		turn, staged, err := store.GetPendingTurn(runID)
		if err != nil {
			return err
		}
		if staged && detail.LastRequest == turn.RequestID {
			if err := store.ClearPendingTurn(runID, turn.RequestID); err != nil {
				return err
			}
			continue
		}
		if !staged {
			pending := pendingReviewMessages(detail)
			if len(pending) == 0 {
				return nil
			}
			lastRequest := pending[len(pending)-1].ID
			if err := store.SetRunning(runID); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "%s is reviewing %s…\n\n", agent.Name, slice.Name)
			result, err := agentreview.RunResult(
				ctx,
				ws.Root,
				slice,
				agent,
				reviewConversationPrompt(slice, detail),
				agentreview.ExecuteCommand,
			)
			if err != nil {
				_, appendErr := store.AppendMessage(runID, reviewrun.RoleSystem, err.Error())
				if appendErr != nil {
					return fmt.Errorf("%w; persist review error message: %v", err, appendErr)
				}
				return persistReviewFailure(store, runID, err)
			}
			findings, err := json.Marshal(result.Findings)
			if err != nil {
				return err
			}
			turn = reviewrun.PendingTurn{
				RequestID: lastRequest, Summary: result.Summary, Findings: findings,
				FindingCount: len(result.Findings),
			}
			if err := store.StageTurn(runID, turn); err != nil {
				return err
			}
		} else if err := store.SetRunning(runID); err != nil {
			return err
		}

		var findings []agentreview.Finding
		if err := json.Unmarshal(turn.Findings, &findings); err != nil {
			return persistReviewFailure(store, runID, err)
		}
		if _, err := store.AppendReviewerMessage(runID, turn.RequestID, turn.Summary); err != nil {
			return persistReviewFailure(store, runID, err)
		}
		fmt.Fprintf(os.Stdout, "%s\n\n%d finding(s)\n", turn.Summary, len(findings))
		if !turn.DeliveryCompleted {
			deliveryID := runID + ":" + turn.RequestID
			if err := deliverReviewFindings(ws, slice, agent.Name, deliveryID, findings); err != nil {
				_, appendErr := store.AppendMessage(runID, reviewrun.RoleSystem, err.Error())
				if appendErr != nil {
					return fmt.Errorf("%w; persist delivery failure: %v", err, appendErr)
				}
				return persistReviewFailure(store, runID, err)
			}
			if err := store.MarkTurnDelivered(runID, turn.RequestID); err != nil {
				return err
			}
		}
		if err := store.SetTurnCompleted(runID, turn.RequestID, turn.FindingCount); err != nil {
			return err
		}
		if err := store.ClearPendingTurn(runID, turn.RequestID); err != nil {
			return err
		}
	}
}

func encodeReviewJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

var reviewRunsCmd = &cobra.Command{
	Use:   "runs [slice]",
	Short: "List persistent agent review conversations",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slice := ""
		if len(args) == 1 {
			slice = args[0]
			if err := validateSliceName(slice); err != nil {
				return err
			}
		}
		runs, err := reviewRunStore().List(slice)
		if err != nil {
			return err
		}
		useJSON, _ := cmd.Flags().GetBool("json")
		if useJSON {
			return encodeReviewJSON(runs)
		}
		if len(runs) == 0 {
			fmt.Fprintln(os.Stdout, "No agent review conversations.")
			return nil
		}
		for _, run := range runs {
			fmt.Fprintf(os.Stdout, "%s  %-9s  %-16s  %s  %d finding(s)\n", run.ID, run.Status, run.Slice, run.Agent, run.FindingCount)
		}
		return nil
	},
}

var reviewShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "Show an agent review conversation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		detail, err := reviewRunStore().Get(args[0])
		if err != nil {
			return err
		}
		useJSON, _ := cmd.Flags().GetBool("json")
		if useJSON {
			return encodeReviewJSON(detail)
		}
		fmt.Fprintf(os.Stdout, "%s · %s · %s · %d finding(s)\n\n", detail.Agent, detail.Status, detail.Slice, detail.FindingCount)
		for _, message := range detail.Messages {
			fmt.Fprintf(os.Stdout, "%s\n%s\n\n", strings.ToUpper(string(message.Role)), message.Body)
		}
		if detail.Error != "" {
			fmt.Fprintf(os.Stdout, "ERROR\n%s\n", detail.Error)
		}
		return nil
	},
}

var reviewMessageCmd = &cobra.Command{
	Use:   "message <run-id>",
	Short: "Send a follow-up message to an agent review conversation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := cmd.Flags().GetString("body")
		if strings.TrimSpace(body) == "" {
			return errors.New("--body is required")
		}
		store := reviewRunStore()
		detail, err := store.Get(args[0])
		if err != nil {
			return err
		}
		if _, err := store.AppendMessage(detail.ID, reviewrun.RoleUser, body); err != nil {
			return err
		}
		current, err := store.Get(detail.ID)
		if err != nil {
			return err
		}
		if current.Status == reviewrun.StatusRunning {
			manager, err := openSessionManager()
			if err != nil {
				return err
			}
			running, err := manager.Busy(cmd.Context(), current.Slice, current.Window)
			if err != nil {
				return err
			}
			if running {
				fmt.Fprintf(os.Stdout, "Message queued for %s review %s\n", current.Agent, current.ID)
				return nil
			}
		}
		if err := store.SetQueued(current.ID); err != nil {
			return err
		}
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return persistReviewFailure(store, current.ID, fmt.Errorf("workspace not found — run `slis init` first: %w", err))
		}
		slice, err := findSlice(ws, current.Slice)
		if err != nil {
			return persistReviewFailure(store, current.ID, err)
		}
		if _, err := agentreview.ResolveAgent(ws.Sessions, current.Agent, exec.LookPath); err != nil {
			return persistReviewFailure(store, current.ID, err)
		}
		if err := startReviewRunWindow(ws, slice, current.Run); err != nil {
			if errors.Is(err, sessionmanager.ErrTerminalBusy) {
				fmt.Fprintf(os.Stdout, "Message queued for %s review %s\n", current.Agent, current.ID)
				return nil
			}
			return persistReviewFailure(store, current.ID, err)
		}
		fmt.Fprintf(os.Stdout, "Message sent to %s review %s\n", current.Agent, current.ID)
		return nil
	},
}

var reviewAttachCmd = &cobra.Command{
	Use:   "attach <run-id>",
	Short: "Attach to an agent review's terminal tab",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		detail, err := reviewRunStore().Get(args[0])
		if err != nil {
			return err
		}
		if detail.Window == "" {
			return fmt.Errorf("review %s has no terminal tab", detail.ID)
		}
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		attach, err := manager.AttachCommand(cmd.Context(), detail.Slice, detail.Window)
		if err != nil {
			return err
		}
		attach.Stdin = cmd.InOrStdin()
		attach.Stdout = cmd.OutOrStdout()
		attach.Stderr = cmd.ErrOrStderr()
		return attach.Run()
	},
}

func init() {
	reviewRunsCmd.Flags().Bool("json", false, "Output as JSON")
	reviewShowCmd.Flags().Bool("json", false, "Output as JSON")
	reviewMessageCmd.Flags().String("body", "", "Follow-up message")
	_ = reviewMessageCmd.MarkFlagRequired("body")
	reviewCmd.AddCommand(reviewRunsCmd)
	reviewCmd.AddCommand(reviewShowCmd)
	reviewCmd.AddCommand(reviewMessageCmd)
	reviewCmd.AddCommand(reviewAttachCmd)
}

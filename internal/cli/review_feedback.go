package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/agentlaunch"
	"github.com/jonnyom/slis/internal/agentreview"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/review"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

type feedbackTarget struct {
	TabID string
	Label string
}

type feedbackSelection struct {
	TabID    string
	NewAgent string
}

func feedbackTargets(group sessionGroupOutput) []feedbackTarget {
	targets := make([]feedbackTarget, 0)
	for _, tab := range group.Tabs {
		if tab.Agent != "" && tab.Kind != sessionmanager.TabKindReview {
			targets = append(targets, feedbackTarget{TabID: tab.ID, Label: tab.Label})
		}
	}
	return targets
}

func nextFeedbackTabID(group sessionmanager.Group) string {
	used := make(map[string]bool)
	for _, tab := range group.Tabs {
		used[tab.ID] = true
	}
	if !used["feedback"] {
		return "feedback"
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("feedback-%d", index)
		if !used[candidate] {
			return candidate
		}
	}
}

func availableFeedbackAgents(sessions config.Sessions) []config.AgentSpec {
	specs := append([]config.AgentSpec(nil), sessions.AgentList()...)
	represented := make(map[string]bool)
	for _, spec := range specs {
		if len(spec.Cmd) > 0 {
			represented[filepath.Base(spec.Cmd[0])] = true
		}
	}
	for _, spec := range agentreview.KnownAgents() {
		if represented[spec.Cmd[0]] {
			continue
		}
		if _, err := exec.LookPath(spec.Cmd[0]); err == nil {
			specs = append(specs, spec)
			represented[spec.Cmd[0]] = true
		}
	}
	return specs
}

func selectFeedbackTarget(cmd *cobra.Command, group sessionGroupOutput, agents []config.AgentSpec) (feedbackSelection, error) {
	tabID, _ := cmd.Flags().GetString("tab")
	newAgent, _ := cmd.Flags().GetString("new-agent")
	if tabID != "" && newAgent != "" {
		return feedbackSelection{}, fmt.Errorf("--tab and --new-agent cannot be used together")
	}
	if tabID != "" {
		for _, target := range feedbackTargets(group) {
			if target.TabID == tabID {
				return feedbackSelection{TabID: tabID}, nil
			}
		}
		return feedbackSelection{}, fmt.Errorf("tab %q has no running coding agent", tabID)
	}
	if newAgent != "" {
		for _, agent := range agents {
			if strings.EqualFold(agent.Name, newAgent) {
				return feedbackSelection{NewAgent: agent.Name}, nil
			}
		}
		return feedbackSelection{}, fmt.Errorf("feedback agent %q is not available", newAgent)
	}

	options := make([]huh.Option[string], 0)
	for _, target := range feedbackTargets(group) {
		options = append(options, huh.NewOption(target.Label, "tab:"+target.TabID))
	}
	if len(agents) > 0 {
		options = append(options, huh.NewOption("New feedback agent…", "new"))
	}
	if len(options) == 0 {
		return feedbackSelection{}, fmt.Errorf("no running or installed coding agents found")
	}
	chosen := ""
	if err := huh.NewSelect[string]().Title("Send feedback to which agent?").Options(options...).Value(&chosen).Run(); err != nil {
		return feedbackSelection{}, err
	}
	if strings.HasPrefix(chosen, "tab:") {
		return feedbackSelection{TabID: strings.TrimPrefix(chosen, "tab:")}, nil
	}
	if len(agents) == 1 {
		return feedbackSelection{NewAgent: agents[0].Name}, nil
	}
	agentOptions := make([]huh.Option[string], 0, len(agents))
	for _, agent := range agents {
		agentOptions = append(agentOptions, huh.NewOption(agent.Name, agent.Name))
	}
	if err := huh.NewSelect[string]().Title("Launch which feedback agent?").Options(agentOptions...).Value(&newAgent).Run(); err != nil {
		return feedbackSelection{}, err
	}
	return feedbackSelection{NewAgent: newAgent}, nil
}

func feedbackAgentCommand(agent config.AgentSpec) string {
	quoted := make([]string, 0, len(agent.Cmd))
	for _, argument := range agent.Cmd {
		quoted = append(quoted, agentlaunch.ShellSingleQuote(argument))
	}
	return strings.Join(quoted, " ")
}

func startFeedbackAgent(ctx context.Context, ws config.Workspace, sl model.Slice, group sessionmanager.Group, manager *sessionmanager.Manager, agent config.AgentSpec) (review.SlisSession, error) {
	tabID := nextFeedbackTabID(group)
	spec := sessionmanager.TabSpec{ID: tabID, Kind: sessionmanager.TabKindAgent, Title: agent.Name, CWD: reviewAgentCwd(sl, ws.Root)}
	if _, err := manager.EnsureTab(ctx, sl.Name, spec); err != nil {
		return review.SlisSession{}, err
	}
	harness := strings.ToLower(filepath.Base(agent.Cmd[0]))
	if err := manager.StartCommand(ctx, sl.Name, spec, agentlaunch.Line(feedbackAgentCommand(agent), sl, ws.Root, harness)); err != nil {
		return review.SlisSession{}, fmt.Errorf("launch feedback agent: %w", err)
	}
	session := review.SlisSession{Manager: manager, AgentCommands: reviewAgentCommands(ws.Sessions), TabID: tabID, Context: ctx}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if session.HasAgent(sl.Name) {
			time.Sleep(time.Second)
			return session, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return review.SlisSession{}, fmt.Errorf("feedback agent %q did not start in slice %q; review comments remain pending", agent.Name, sl.Name)
}

func automaticFeedbackSession(ctx context.Context, ws config.Workspace, sl model.Slice, manager *sessionmanager.Manager) (review.SlisSession, error) {
	group, err := manager.Ensure(ctx, sl.Name, reviewSessionMembers(sl), sessionmanager.LayoutOptions{Root: ws.Root, Layout: ws.Sessions.Layout})
	if err != nil {
		return review.SlisSession{}, err
	}
	runtimes, err := manager.Runtimes(ctx, []sessionmanager.Group{group})
	if err != nil {
		return review.SlisSession{}, err
	}
	liveGroup := sessionGroupForOutputWithRuntime(group, runtimes, detectableAgentSpecs(ws.Sessions))
	targets := feedbackTargets(liveGroup)
	if len(targets) > 0 {
		target := targets[0]
		for _, candidate := range targets {
			if candidate.TabID == group.ActiveTabID {
				target = candidate
				break
			}
		}
		return review.SlisSession{Manager: manager, AgentCommands: reviewAgentCommands(ws.Sessions), TabID: target.TabID, Context: ctx}, nil
	}
	agents := availableFeedbackAgents(ws.Sessions)
	if len(agents) == 0 {
		return review.SlisSession{}, fmt.Errorf("no running or installed coding agents found")
	}
	return startFeedbackAgent(ctx, ws, sl, group, manager, agents[0])
}

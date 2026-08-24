package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/agentreview"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/review"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

func detectableAgentSpecs(sessions config.Sessions) []config.AgentSpec {
	specs := append([]config.AgentSpec(nil), sessions.AgentList()...)
	represented := make(map[string]bool)
	for _, spec := range specs {
		if len(spec.Cmd) > 0 {
			represented[filepath.Base(spec.Cmd[0])] = true
		}
	}
	for _, spec := range agentreview.KnownAgents() {
		if !represented[spec.Cmd[0]] {
			specs = append(specs, spec)
		}
	}
	return specs
}

func sessionGroupForOutputWithRuntime(group sessionmanager.Group, runtimes []sessionmanager.TabRuntime, agents []config.AgentSpec) sessionGroupOutput {
	runtimeByTab := make(map[string]sessionmanager.TabRuntime)
	for _, runtime := range runtimes {
		if runtime.GroupID == group.ID {
			runtimeByTab[runtime.TabID] = runtime
		}
	}
	output := sessionGroupForOutput(group)
	counts := make(map[string]int)
	for index := range output.Tabs {
		tab := &output.Tabs[index]
		runtime := runtimeByTab[tab.ID]
		tab.Agent = runtimeAgentName(runtime, agents)
		tab.CurrentDirectory = runtimeCurrentDirectory(runtime, tab.CWD)
		if tab.Agent == "" {
			tab.Label = displayDirectory(tab.CurrentDirectory)
			continue
		}
		count := counts[tab.Agent]
		tab.Label = tab.Agent
		if count > 0 {
			tab.Label = fmt.Sprintf("%s (%d)", tab.Agent, count)
		}
		counts[tab.Agent] = count + 1
	}
	return output
}

func runtimeAgentName(runtime sessionmanager.TabRuntime, agents []config.AgentSpec) string {
	for _, agent := range agents {
		if len(agent.Cmd) == 0 {
			continue
		}
		command := strings.Join(agent.Cmd, " ")
		for _, process := range runtime.Processes {
			if review.CommandLineMatchesAgent("", process.Cmd, []string{command}) {
				return agent.Name
			}
		}
	}
	return ""
}

func runtimeCurrentDirectory(runtime sessionmanager.TabRuntime, fallback string) string {
	parents := make(map[int]bool)
	for _, process := range runtime.Processes {
		parents[process.PPID] = true
	}
	for _, process := range runtime.Processes {
		if process.CWD != "" && !parents[process.PID] {
			return process.CWD
		}
	}
	for _, process := range runtime.Processes {
		if process.CWD != "" {
			return process.CWD
		}
	}
	return fallback
}

func displayDirectory(directory string) string {
	home, err := os.UserHomeDir()
	if err == nil && (directory == home || strings.HasPrefix(directory, home+string(os.PathSeparator))) {
		return "~" + strings.TrimPrefix(directory, home)
	}
	return directory
}

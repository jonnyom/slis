package hooks

import "github.com/jonnyom/slis/internal/model"

// backgroundTask is one entry of the in-flight work Claude Code reports on
// Stop and SubagentStop.
type backgroundTask struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// sessionTransition is what one hook event does to its session's status.
type sessionTransition struct {
	status     model.SessionStatus
	subagents  int
	endSession bool
	unchanged  bool
}

var keepStatus = sessionTransition{unchanged: true}

var working = sessionTransition{status: model.SessRunning}

// transitionFor maps a hook event, given the session's previous status, to its
// next status:
//
//	UserPromptSubmit, main-thread PostToolUse → running
//	Notification needing the user              → waiting-input
//	Notification idle_prompt                   → done
//	Stop                                       → subagents (N still working) or done
//	SubagentStop while main thread is idle     → subagents (N left) or done
//	SessionEnd                                 → session removed
//
// Tool events fired inside a sub-agent carry an agent_id and leave the main
// thread's status alone; so does a SubagentStop while the main thread works.
func transitionFor(event string, input hookInput, previous model.SessionStatus) sessionTransition {
	switch event {
	case "SessionEnd":
		return sessionTransition{endSession: true}
	case "Notification":
		return notificationTransition(input.NotificationType)
	case "Stop":
		return idleTransition(runningSubagents(input.BackgroundTasks, ""))
	case "SubagentStop":
		if previous != model.SessSubagents {
			return keepStatus
		}
		return idleTransition(runningSubagents(input.BackgroundTasks, input.AgentID))
	case "PostToolUse":
		if input.AgentID != "" {
			return keepStatus
		}
		return working
	default:
		return working
	}
}

func notificationTransition(notificationType string) sessionTransition {
	switch notificationType {
	case "", "permission_prompt", "elicitation_dialog", "agent_needs_input":
		return sessionTransition{status: model.SessWaitingInput}
	case "idle_prompt":
		return sessionTransition{status: model.SessDone}
	default:
		return keepStatus
	}
}

func idleTransition(subagents int) sessionTransition {
	if subagents > 0 {
		return sessionTransition{status: model.SessSubagents, subagents: subagents}
	}
	return sessionTransition{status: model.SessDone}
}

var finishedTaskStatuses = map[string]bool{
	"completed": true,
	"failed":    true,
	"killed":    true,
	"stopped":   true,
	"cancelled": true,
}

func runningSubagents(tasks []backgroundTask, stoppedAgentID string) int {
	count := 0
	for _, task := range tasks {
		if task.Type == "subagent" && task.ID != stoppedAgentID && !finishedTaskStatuses[task.Status] {
			count++
		}
	}
	return count
}

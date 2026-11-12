package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/review"
	sessionmanager "github.com/jonnyom/slis/internal/session"
	"github.com/spf13/cobra"
)

type reapCandidate struct {
	Slice            string   `json:"slice"`
	Tab              string   `json:"tab"`
	IdleHours        int      `json:"idle_hours"`
	MemoryMB         int      `json:"memory_mb"`
	ResumeSessionIDs []string `json:"resume_session_ids"`
}

var sessionReapIdle time.Duration
var sessionReapYes bool
var sessionReapJSON bool

var sessionReapCmd = &cobra.Command{
	Use:   "reap",
	Short: "Close Claude tabs whose slice has been quiet for a while",
	Long: "Lists Claude tabs in slices whose Claude sessions have all been quiet for longer than --idle " +
		"(no running or sub-agent work). Without --yes nothing is closed. " +
		"Closed conversations can be reopened with `claude --resume <id>`.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		candidates, reapErr := reapIdleClaudeTabs(cmd.Context(), manager, sessionReapIdle, sessionReapYes)
		if candidates == nil {
			return reapErr
		}
		if sessionReapJSON {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(candidates); err != nil {
				return err
			}
		} else {
			printReapCandidates(cmd, candidates, sessionReapYes)
		}
		return reapErr
	},
}

func reapIdleClaudeTabs(ctx context.Context, manager *sessionmanager.Manager, idleFor time.Duration, closeTabs bool) ([]reapCandidate, error) {
	groups, err := sessionmanager.OpenStore(config.StatePaths().StateDir).List()
	if err != nil {
		return nil, err
	}
	runtimes, err := manager.Runtimes(ctx, groups)
	if err != nil {
		return nil, err
	}
	eventsDirectory := config.StatePaths().EventsDir
	recordsBySlice := make(map[string][]notify.Status, len(groups))
	for _, group := range groups {
		recordsBySlice[group.ID] = notify.SessionRecords(eventsDirectory, group.ID)
	}
	candidates := idleClaudeTabs(groups, runtimes, recordsBySlice, time.Now(), idleFor)
	if !closeTabs {
		return candidates, nil
	}
	var closeErrors []error
	for _, candidate := range candidates {
		if err := manager.KillTab(ctx, candidate.Slice, candidate.Tab); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close %s/%s: %w", candidate.Slice, candidate.Tab, err))
		}
	}
	return candidates, errors.Join(closeErrors...)
}

func reapIdleClaudeTabsEvery(ctx context.Context, manager *sessionmanager.Manager, idleFor, interval time.Duration, report io.Writer) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		candidates, err := reapIdleClaudeTabs(ctx, manager, idleFor, true)
		if err != nil {
			fmt.Fprintln(report, "rpc: reap idle Claude tabs:", err)
		}
		for _, candidate := range candidates {
			fmt.Fprintf(report, "rpc: reaped %s/%s (quiet %dh)\n", candidate.Slice, candidate.Tab, candidate.IdleHours)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func idleClaudeTabs(groups []sessionmanager.Group, runtimes []sessionmanager.TabRuntime, recordsBySlice map[string][]notify.Status, now time.Time, idleFor time.Duration) []reapCandidate {
	runtimeByTab := make(map[string]sessionmanager.TabRuntime, len(runtimes))
	for _, runtime := range runtimes {
		runtimeByTab[runtime.GroupID+"\x00"+runtime.TabID] = runtime
	}
	candidates := make([]reapCandidate, 0)
	for _, group := range groups {
		lastActivity, quiet := sliceQuietSince(recordsBySlice[group.ID], now)
		if !quiet || now.Sub(lastActivity) < idleFor {
			continue
		}
		for _, tab := range group.Tabs {
			runtime := runtimeByTab[group.ID+"\x00"+tab.ID]
			if !runsClaude(runtime.Processes) {
				continue
			}
			candidates = append(candidates, reapCandidate{
				Slice:            group.ID,
				Tab:              tab.ID,
				IdleHours:        int(now.Sub(lastActivity).Hours()),
				MemoryMB:         int(math.Round(treeMemoryMB(runtime.Processes))),
				ResumeSessionIDs: sessionIDs(recordsBySlice[group.ID]),
			})
		}
	}
	return candidates
}

func sliceQuietSince(records []notify.Status, now time.Time) (time.Time, bool) {
	if len(records) == 0 {
		return time.Time{}, false
	}
	var latest int64
	for _, record := range records {
		if stillWorking(record, now) {
			return time.Time{}, false
		}
		latest = max(latest, record.TimeNS)
	}
	return time.Unix(0, latest), true
}

func stillWorking(record notify.Status, now time.Time) bool {
	status := notify.ParseStatus(record.Status)
	return (status == model.SessRunning || status == model.SessSubagents) && !record.Expired(now)
}

func runsClaude(processes []proc.ProcInfo) bool {
	for _, process := range processes {
		if review.CommandLineMatchesAgent("", process.Cmd, []string{"claude"}) {
			return true
		}
	}
	return false
}

func treeMemoryMB(processes []proc.ProcInfo) float64 {
	var total float64
	for _, process := range processes {
		total += process.MemMB
	}
	return total
}

func sessionIDs(records []notify.Status) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.SessionID != "" {
			ids = append(ids, record.SessionID)
		}
	}
	sort.Strings(ids)
	return slices.Compact(ids)
}

func printReapCandidates(cmd *cobra.Command, candidates []reapCandidate, closed bool) {
	out := cmd.OutOrStdout()
	if len(candidates) == 0 {
		fmt.Fprintf(out, "No Claude tabs quiet for more than %s.\n", sessionReapIdle)
		return
	}
	verb := "Would close"
	if closed {
		verb = "Closed"
	}
	for _, candidate := range candidates {
		fmt.Fprintf(out, "%s %s/%s  quiet %dh  %d MB\n", verb, candidate.Slice, candidate.Tab, candidate.IdleHours, candidate.MemoryMB)
		for _, id := range candidate.ResumeSessionIDs {
			fmt.Fprintf(out, "    resume: claude --resume %s\n", id)
		}
	}
	if !closed {
		fmt.Fprintln(out, "Run again with --yes to close these tabs.")
	}
}

func init() {
	sessionReapCmd.Flags().DurationVar(&sessionReapIdle, "idle", 24*time.Hour, "close tabs whose slice has been quiet for longer than this")
	sessionReapCmd.Flags().BoolVar(&sessionReapYes, "yes", false, "close the listed tabs (default only lists them)")
	sessionReapCmd.Flags().BoolVar(&sessionReapJSON, "json", false, "print the tabs as JSON")
	sessionCmd.AddCommand(sessionReapCmd)
}

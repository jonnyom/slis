package tui

import (
	"fmt"
	"strings"
)

// renderConflictOverlay shows the cross-slice conflict radar: every (repo, file)
// changed by more than one slice, plus any scopes the radar could not read. The
// list is scrollable with j/k (offset = m.conflictScroll). Honest framing: file
// overlap is a heads-up, not a guaranteed git merge conflict, and it reflects
// committed changes only.
func renderConflictOverlay(m Model) string {
	var sb strings.Builder
	sb.WriteString(cockpitHeaderStyle.Render("Conflict radar — files changed by >1 slice") + "\n\n")

	idx := m.conflicts
	if idx == nil || (len(idx.Overlaps) == 0 && len(idx.Incomplete) == 0) {
		sb.WriteString(overviewStyle.Render("No cross-slice conflicts — no file is changed by more than one slice.") + "\n\n")
		sb.WriteString(helpDescStyle.Render("Committed changes only (uncommitted worktree edits are invisible).") + "\n")
		sb.WriteString(helpDescStyle.Render("Press ! or esc to close"))
		return helpBoxStyle.Render(sb.String())
	}

	if len(idx.Overlaps) > 0 {
		lines := make([]string, 0, len(idx.Overlaps))
		for _, o := range idx.Overlaps {
			lines = append(lines, fmt.Sprintf("%s  %s  %s",
				panelTitleFocusStyle.Render(o.Repo),
				o.Path,
				helpDescStyle.Render("← "+strings.Join(o.Slices, ", "))))
		}

		maxRows := m.height - 14
		if maxRows < 5 {
			maxRows = 5
		}
		start := clamp(m.conflictScroll, 0, max(0, len(lines)-maxRows))
		end := min(start+maxRows, len(lines))
		if start > 0 {
			sb.WriteString(helpDescStyle.Render(fmt.Sprintf("  ↑ %d more above", start)) + "\n")
		}
		for _, ln := range lines[start:end] {
			sb.WriteString("  " + ln + "\n")
		}
		if end < len(lines) {
			sb.WriteString(helpDescStyle.Render(fmt.Sprintf("  ↓ %d more below", len(lines)-end)) + "\n")
		}
	} else {
		sb.WriteString(overviewStyle.Render("No overlaps among the slices the radar could read.") + "\n")
	}

	if len(idx.Incomplete) > 0 {
		sb.WriteString("\n" + waitStyle.Render("radar incomplete (diff unavailable, may hide conflicts) for: "+
			strings.Join(idx.Incomplete, ", ")) + "\n")
	}

	sb.WriteString("\n" + helpDescStyle.Render("File overlap is a heads-up, not a guaranteed merge conflict. Committed changes only.") + "\n")
	sb.WriteString(helpDescStyle.Render("j/k scroll · ! or esc to close"))
	return helpBoxStyle.Render(sb.String())
}

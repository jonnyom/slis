package tui

import (
	"fmt"
	"path/filepath"
	"strings"
)

// renderCandidateOverlay lists the unmanaged worktrees slis discovered but did
// NOT ingest (opt-in). Each row can be imported ([i]) or ignored ([x]); esc
// closes. j/k move the selection.
func renderCandidateOverlay(m Model) string {
	var sb strings.Builder
	sb.WriteString(cockpitHeaderStyle.Render("New worktrees — import to manage as slices") + "\n\n")

	if len(m.candidates) == 0 {
		sb.WriteString(overviewStyle.Render("No new worktrees — everything is managed or ignored.") + "\n\n")
		sb.WriteString(helpDescStyle.Render("Press esc to close"))
		return helpBoxStyle.Render(sb.String())
	}

	for i, c := range m.candidates {
		marker := "  "
		if i == m.candidateSel {
			marker = cursorBar.Render("▎") + " "
		}
		label := fmt.Sprintf("%s  %s  %s",
			c.Slice,
			helpDescStyle.Render(c.Repo+" · "+c.Branch),
			overviewStyle.Render(filepath.Dir(c.Path)))
		if i == m.candidateSel {
			label = focusStyle.Render(c.Slice) + "  " +
				helpDescStyle.Render(c.Repo+" · "+c.Branch) + "  " +
				overviewStyle.Render(filepath.Dir(c.Path))
		}
		sb.WriteString(marker + label + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpDescStyle.Render("[i] import   [x] ignore (add to workspace.yaml)   [j/k] move   [esc] close"))
	return helpBoxStyle.Render(sb.String())
}

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/model"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
	focusStyle  = lipgloss.NewStyle().Bold(true)
	normalStyle = lipgloss.NewStyle()
	footerStyle = lipgloss.NewStyle().Faint(true)
)

// renderSliceList renders the full slice-list view, including title and footer hint.
func renderSliceList(m Model) string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("slis — slices"))
	sb.WriteString("\n\n")

	if len(m.slices) == 0 {
		sb.WriteString("No slices found. Run 'slis init' to set up your workspace.\n")
	} else {
		for i, s := range m.slices {
			sb.WriteString(renderSliceRow(i, s, i == m.focus))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(footerStyle.Render("[j/k] move  [r] refresh  [q] quit"))
	sb.WriteString("\n")

	return sb.String()
}

// renderSliceRow renders a single slice row with focus and active markers.
func renderSliceRow(idx int, s model.Slice, focused bool) string {
	// Focus marker: ">" when focused, " " otherwise.
	focusMarker := " "
	if focused {
		focusMarker = ">"
	}

	// Active marker: "●" when the slice is the currently activated one.
	activeMarker := " "
	if s.Active {
		activeMarker = "●"
	}

	// Compact repo summary: repo(branch), …
	repos := s.Repos()
	parts := make([]string, 0, len(repos))
	for _, repo := range repos {
		m := s.Members[repo]
		parts = append(parts, repo+"("+m.Branch+")")
	}
	repoSummary := strings.Join(parts, ", ")

	line := focusMarker + " " + activeMarker + " " + s.Name
	if repoSummary != "" {
		line += "  " + repoSummary
	}

	if focused {
		return focusStyle.Render(line)
	}
	return normalStyle.Render(line)
}

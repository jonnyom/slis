package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Binding represents a single key binding with display help text.
type Binding struct {
	Keys []string
	Help string
}

// browserBindings is the keymap for the dashboard hub (landing screen).
var browserBindings = []Binding{
	{[]string{"tab"}, "switch focus: States rail ⇄ Slices list"},
	{[]string{"j", "k"}, "move within the focused panel (slice, or state filter)"},
	{[]string{"1-6"}, "jump to a state filter (All / Needs you / In review / Ready / …)"},
	{[]string{"g", "G"}, "first / last slice"},
	{[]string{"enter", "l"}, "open slice cockpit"},
	{[]string{"space"}, "select / deselect (for grouping)"},
	{[]string{"m"}, "group selected slices under a name"},
	{[]string{"u"}, "ungroup the focused slice"},
	{[]string{"d"}, "clear a finished slice (remove worktrees/branches/session)"},
	{[]string{"w"}, "set as live (swap into primaries) / deactivate"},
	{[]string{"R"}, "restack / sync the slice's Graphite stacks"},
	{[]string{"Y"}, "copy PR-stack markdown to clipboard"},
	{[]string{"/"}, "search by name"},
	{[]string{"a"}, "attach tmux session"},
	{[]string{"P"}, "processes overlay (all slices)"},
	{[]string{"r"}, "refresh"},
	{[]string{"?"}, "help"},
	{[]string{"q"}, "quit"},
}

// cockpitBindings is the keymap for the single-slice cockpit.
var cockpitBindings = []Binding{
	{[]string{"tab", "shift+tab"}, "focus next / previous panel"},
	{[]string{"1", "2", "3", "4"}, "jump to panel"},
	{[]string{"j", "k"}, "select within focused panel"},
	{[]string{"⏶⏷", "^d/^u"}, "scroll right pane"},
	{[]string{"g", "G"}, "top / bottom of right pane"},
	{[]string{"t"}, "toggle split / unified diff (Stack panel)"},
	{[]string{"enter"}, "zoom right pane (toggle)"},
	{[]string{"w"}, "set as live (swap into primaries) / deactivate"},
	{[]string{"d"}, "clear this finished slice (worktrees/branches/session)"},
	{[]string{"R"}, "restack / sync the slice's Graphite stacks"},
	{[]string{"s"}, "summary (toggle); S forces AI summary"},
	{[]string{"a"}, "attach tmux session"},
	{[]string{"o"}, "open worktree in editor"},
	{[]string{"y", "Y"}, "yank diff / PR-stack markdown"},
	{[]string{"c"}, "PR comments overlay"},
	{[]string{"r"}, "refresh session capture (Session panel)"},
	{[]string{"F"}, "fix CI (point Claude at failing CI)"},
	{[]string{"x"}, "kill selected process (Processes panel)"},
	{[]string{"esc", "h"}, "back to browser"},
	{[]string{"?"}, "help"},
	{[]string{"q"}, "quit"},
}

var (
	helpBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			BorderForeground(lipgloss.Color("62"))

	helpKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	helpDescStyle = lipgloss.NewStyle().Faint(true)
)

// renderHelp formats the bindings for the active view into a help overlay box.
func renderHelp(view viewMode) string {
	bindings := browserBindings
	title := "slis — Browser shortcuts"
	if view == viewCockpit {
		bindings = cockpitBindings
		title = "slis — Cockpit shortcuts"
	}

	var sb strings.Builder
	sb.WriteString(title + "\n\n")
	for _, b := range bindings {
		keys := strings.Join(b.Keys, " / ")
		sb.WriteString(fmt.Sprintf("  %-22s %s\n",
			helpKeyStyle.Render(keys),
			helpDescStyle.Render(b.Help),
		))
	}
	sb.WriteString("\n")
	sb.WriteString(helpDescStyle.Render("Press ? or esc to close"))

	return helpBoxStyle.Render(sb.String())
}

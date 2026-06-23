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

// browserBindings is the keymap for the slice browser (landing screen).
var browserBindings = []Binding{
	{[]string{"j", "↓"}, "next slice"},
	{[]string{"k", "↑"}, "previous slice"},
	{[]string{"g", "G"}, "first / last slice"},
	{[]string{"enter", "l"}, "open slice cockpit"},
	{[]string{"w"}, "set as live (swap into primaries) / deactivate"},
	{[]string{"Y"}, "copy PR-stack markdown to clipboard"},
	{[]string{"/"}, "filter by name"},
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

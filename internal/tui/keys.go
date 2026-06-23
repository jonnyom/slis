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

// Bindings is the data-driven keymap for the slis TUI.
var Bindings = []Binding{
	{[]string{"j", "↓"}, "down"},
	{[]string{"k", "↑"}, "up"},
	{[]string{"tab", "l"}, "next tab"},
	{[]string{"shift+tab", "h"}, "prev tab"},
	{[]string{"a"}, "attach tmux session"},
	{[]string{"P"}, "processes overlay"},
	{[]string{"Y"}, "copy PR stack markdown (Stack tab)"},
	{[]string{"c"}, "comments overlay (Stack tab)"},
	{[]string{"F"}, "fix CI (Stack tab)"},
	{[]string{"s"}, "AI summary (Summary tab)"},
	{[]string{"o"}, "open in editor (Changes tab)"},
	{[]string{"y"}, "copy patch (Changes tab)"},
	{[]string{"pgup", "pgdn"}, "scroll (Changes tab)"},
	{[]string{"r"}, "refresh"},
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

// renderHelp formats Bindings into a help overlay box.
func renderHelp() string {
	var sb strings.Builder
	sb.WriteString("Keyboard shortcuts\n\n")

	for _, b := range Bindings {
		keys := strings.Join(b.Keys, " / ")
		line := fmt.Sprintf("  %-20s %s\n",
			helpKeyStyle.Render(keys),
			helpDescStyle.Render(b.Help),
		)
		sb.WriteString(line)
	}

	sb.WriteString("\n")
	sb.WriteString(helpDescStyle.Render("Press ? to close"))

	return helpBoxStyle.Render(sb.String())
}

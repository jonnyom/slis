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
	{[]string{"n", "N"}, "jump to next / prev slice needing attention (triage)"},
	{[]string{"1-8"}, "jump to a state filter (Inbox is 8; All / Needs you / Ready / …)"},
	{[]string{"g", "G"}, "first / last slice"},
	{[]string{"enter", "l"}, "open slice cockpit"},
	{[]string{"^d/^u", "pgdn/pgup"}, "scroll the preview pane (diff / session output)"},
	{[]string{"c"}, "create a new slice (worktrees across repos)"},
	{[]string{"i"}, "adopt an existing branch as a slice (interactive picker)"},
	{[]string{"C"}, "launch the agent (claude) in the session + attach"},
	{[]string{"a"}, "attach tmux session  ·  detach with C-b d (not Ctrl-D)"},
	{[]string{"w"}, "set as live (swap into primaries) / deactivate"},
	{[]string{"R"}, "stack actions: restack / submit / merge (Graphite) / sync"},
	{[]string{"d"}, "clear finished slice(s) — selection if any, else focused"},
	{[]string{"space", "A"}, "select one / select all visible (for batch ops)"},
	{[]string{"m", "u"}, "group selected slices / ungroup focused"},
	{[]string{"Y"}, "copy PR-stack markdown to clipboard"},
	{[]string{"/"}, "search by name"},
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
	{[]string{"b"}, "toggle diff base: this branch (vs parent) ↔ whole feature (vs trunk)"},
	{[]string{"enter"}, "zoom right pane (toggle)"},
	{[]string{"w"}, "set as live (swap into primaries) / deactivate"},
	{[]string{"d"}, "clear this finished slice (worktrees/branches/session)"},
	{[]string{"R"}, "stack actions: restack / submit / merge (Graphite) / sync"},
	{[]string{"s"}, "summary (toggle); S forces AI summary"},
	{[]string{"a"}, "attach tmux session  ·  detach with C-b d (not Ctrl-D)"},
	{[]string{"C"}, "launch the agent (claude) in the session + attach"},
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

	helpKeyStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
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
	sb.WriteString(helpDescStyle.Render("Slice glyph:  ") +
		waitStyle.Render("⏸") + helpDescStyle.Render(" waiting on you   ") +
		doneStyle.Render("✦") + helpDescStyle.Render(" finished (your move)   ") +
		helpDescStyle.Render("❌ attention   ") +
		liveStyle.Render("●") + helpDescStyle.Render(" live (swapped in)   ") +
		helpDescStyle.Render("● running   ") +
		readyStyle.Render("♻") + helpDescStyle.Render(" ready   ") +
		syncedStyle.Render("✓") + helpDescStyle.Render(" in review   ") +
		overviewStyle.Render("·") + helpDescStyle.Render(" idle") + "\n")
	sb.WriteString(helpDescStyle.Render("In a session: detach with C-b d (prefix, then d) — Ctrl-D sends EOF and quits Claude.\n"))
	sb.WriteString(helpDescStyle.Render("Press ? or esc to close"))

	return helpBoxStyle.Render(sb.String())
}

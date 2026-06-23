package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/summary"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	focusStyle    = lipgloss.NewStyle().Bold(true)
	footerStyle   = lipgloss.NewStyle().Faint(true)
	cursorBar     = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	syncedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	overviewStyle = lipgloss.NewStyle().Faint(true)
	headerStyle   = lipgloss.NewStyle().Faint(true)
)

// sliceCard is the lazily-computed browser summary of a slice: what it's about
// (latest commit subject), its diffstat, commit count, and stack health. PR
// information is overlaid at render time from the (separately-loaded) PR cache.
type sliceCard struct {
	overview   string // newest commit subject across the slice's repos
	added      int
	deleted    int
	commits    int
	restack    int  // needs-restack branches across the slice's lineages
	stackKnown bool // Graphite data was available for at least one repo
}

// cardLoadedMsg is delivered when a slice's browser card has been computed.
type cardLoadedMsg struct {
	slice string
	card  sliceCard
}

// loadCardCmd computes a slice's browser card off the UI goroutine: commit
// subjects/count, diffstat (numstat only), and stack health — each per repo,
// using auto-detected trunks.
func loadCardCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		var card sliceCard

		byRepo, _ := summary.CommitSummary(sl, "")
		for _, repo := range sl.Repos() {
			subs := byRepo[repo]
			card.commits += len(subs)
			if card.overview == "" && len(subs) > 0 {
				card.overview = subs[0] // newest first
			}
		}

		stats, _ := diff.SliceStat(sl, "")
		for _, rd := range stats {
			card.added += rd.TotalAdded()
			card.deleted += rd.TotalDeleted()
		}

		for _, repo := range sl.Repos() {
			member := sl.Members[repo]
			if member.WorktreePath == "" {
				continue
			}
			st, err := gt.ReadState(member.WorktreePath)
			if err != nil || len(st) == 0 {
				continue
			}
			lineage := st.Lineage(member.Branch)
			if len(lineage) == 0 {
				continue // branch isn't in a Graphite stack — leave health unknown
			}
			card.stackKnown = true
			for _, b := range lineage {
				if b.NeedsRestack {
					card.restack++
				}
			}
		}

		return cardLoadedMsg{slice: sl.Name, card: card}
	}
}

// batchLoadCards loads cards for every slice not yet loaded/loading.
func (m *Model) batchLoadCards() tea.Cmd {
	var cmds []tea.Cmd
	for _, sl := range m.slices {
		if _, ok := m.cards[sl.Name]; ok {
			continue
		}
		if m.cardLoading[sl.Name] {
			continue
		}
		m.cardLoading[sl.Name] = true
		cmds = append(cmds, loadCardCmd(sl))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// filteredIndices returns the indices into m.slices that match the active
// filter, in order. An empty filter matches everything.
func (m Model) filteredIndices() []int {
	var out []int
	f := strings.ToLower(m.filter)
	for i, s := range m.slices {
		if f == "" || strings.Contains(strings.ToLower(s.Name), f) {
			out = append(out, i)
		}
	}
	return out
}

// posInFiltered returns the position of m.focus within the filtered list, or -1.
func posInFiltered(idxs []int, focus int) int {
	for p, i := range idxs {
		if i == focus {
			return p
		}
	}
	return -1
}

// renderBrowser renders the slice browser (landing screen).
func renderBrowser(m Model) string {
	idxs := m.filteredIndices()

	// Header.
	var head strings.Builder
	head.WriteString(titleStyle.Render("slis"))
	head.WriteString(headerStyle.Render(fmt.Sprintf("  %d slices", len(m.slices))))
	if names := repoNames(m.slices); names != "" {
		head.WriteString(headerStyle.Render("  ▏ " + names))
	}
	if m.filtering || m.filter != "" {
		head.WriteString(headerStyle.Render("   /") + m.filter)
		if m.filtering {
			head.WriteString("▏")
		}
	}
	header := clip(head.String(), m.width)

	footer := footerStyle.Render("[enter] open  [a] attach  [/] filter  [r] refresh  [?] help  ○none ●run ⏸wait ✓done")
	footer = clip(footer, m.width)

	if len(m.slices) == 0 {
		return header + "\n\n  No slices found. Run 'slis init' to set up your workspace.\n\n" + footer
	}
	if len(idxs) == 0 {
		return header + "\n\n  " + overviewStyle.Render("no slices match /"+m.filter) + "\n\n" + footer
	}

	// Two lines per slice; window around the focused row. When height is unknown
	// (before the first WindowSizeMsg) show every match.
	visible := len(idxs)
	if m.height > 0 {
		bodyH := m.height - 3
		if bodyH < 2 {
			bodyH = 2
		}
		visible = bodyH / 2
		if visible < 1 {
			visible = 1
		}
	}
	pos := posInFiltered(idxs, m.focus)
	if pos < 0 {
		pos = 0
	}
	start := pos - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(idxs) {
		end = len(idxs)
		start = end - visible
		if start < 0 {
			start = 0
		}
	}

	var body strings.Builder
	for _, i := range idxs[start:end] {
		body.WriteString(renderCard(m, i, i == m.focus))
	}

	return header + "\n\n" + strings.TrimRight(body.String(), "\n") + "\n" + footer
}

// repoNames returns the distinct member-repo names across all slices, joined.
func repoNames(slices []model.Slice) string {
	seen := map[string]bool{}
	var names []string
	for _, s := range slices {
		for _, r := range s.Repos() {
			if !seen[r] {
				seen[r] = true
				names = append(names, r)
			}
		}
	}
	return strings.Join(names, " · ")
}

// renderCard renders a single slice as two lines.
func renderCard(m Model, idx int, focused bool) string {
	s := m.slices[idx]

	cursor := "  "
	if focused {
		cursor = cursorBar.Render("▎") + " "
	}

	status := model.SessNone
	if m.sessionStatus != nil {
		status = m.sessionStatus[s.Name]
	}

	nameW := clamp(m.width-46, 18, 44)
	name := clip(s.Name, nameW)
	name = lipgloss.NewStyle().Width(nameW).Render(name)
	if focused {
		name = focusStyle.Render(name)
	}

	line1 := fmt.Sprintf("%s%s  %s  %s  %s  %s",
		cursor, name, repoDots(s), stackBadge(m, s), prBadge(m, s), sessionBadge(status))

	line2 := "   " + overviewStyle.Render(cardOverview(m, s))

	return clip(line1, m.width) + "\n" + clip(line2, m.width) + "\n"
}

// repoDots renders a fixed-width member indicator: up to three ◆ then a count.
func repoDots(s model.Slice) string {
	n := len(s.Members)
	filled := n
	if filled > 3 {
		filled = 3
	}
	dots := strings.Repeat("◆", filled) + strings.Repeat("·", 3-filled)
	return fmt.Sprintf("%s %d", dots, n)
}

// stackBadge renders the slice's stack health from its card.
func stackBadge(m Model, s model.Slice) string {
	c, ok := m.cards[s.Name]
	if !ok || !c.stackKnown {
		return overviewStyle.Render("· · · · ·")
	}
	if c.restack == 0 {
		return syncedStyle.Render("✓ synced  ")
	}
	return needsRestackStyle.Render(fmt.Sprintf("⚠ %d restack", c.restack))
}

// prBadge renders PR state for the slice (first repo with a PR). PRs are loaded
// lazily for the focused slice, so unfocused rows usually show "—".
func prBadge(m Model, s model.Slice) string {
	slicePRs, ok := m.prs[s.Name]
	if m.prLoading[s.Name] && !ok {
		return overviewStyle.Render("PR…    ")
	}
	if !ok {
		return overviewStyle.Render("—      ")
	}
	for _, repo := range s.Repos() {
		if pr := slicePRs[repo]; pr != nil {
			overall, _, _, _ := pr.CISummary()
			return fmt.Sprintf("#%d %s", pr.Number, forge.CIEmoji(overall))
		}
	}
	return overviewStyle.Render("no PR  ")
}

// cardOverview returns the one-line "what is this slice about" text: the PR
// title if a PR is loaded, else the newest commit subject, plus diffstat.
func cardOverview(m Model, s model.Slice) string {
	c, loaded := m.cards[s.Name]
	if !loaded {
		return "loading…"
	}

	desc := c.overview
	if slicePRs, ok := m.prs[s.Name]; ok {
		for _, repo := range s.Repos() {
			if pr := slicePRs[repo]; pr != nil && pr.Title != "" {
				desc = pr.Title
				break
			}
		}
	}
	if desc == "" {
		desc = "(no commits vs trunk)"
	}

	stat := fmt.Sprintf(" · +%d −%d · %d commit%s", c.added, c.deleted, c.commits, plural(c.commits))
	// Reserve room for the stat tail so the description truncates, not the stats.
	descW := clamp(m.width-len(stat)-4, 10, m.width)
	return clip(desc, descW) + stat
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// updateBrowserKeys handles key events while the browser is showing.
func (m Model) updateBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.Type {
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.filter += string(msg.Runes)
		}
		m.snapFocusToFilter()
		return m, m.loadFocusedPRs()
	}

	idxs := m.filteredIndices()
	pos := posInFiltered(idxs, m.focus)

	switch msg.String() {
	case "/":
		m.filtering = true
		return m, nil
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.snapFocusToFilter()
		}
		return m, nil
	case "j", "down":
		if pos >= 0 && pos < len(idxs)-1 {
			m.focus = idxs[pos+1]
			return m, m.loadFocusedPRs()
		}
	case "k", "up":
		if pos > 0 {
			m.focus = idxs[pos-1]
			return m, m.loadFocusedPRs()
		}
	case "g":
		if len(idxs) > 0 {
			m.focus = idxs[0]
			return m, m.loadFocusedPRs()
		}
	case "G":
		if len(idxs) > 0 {
			m.focus = idxs[len(idxs)-1]
			return m, m.loadFocusedPRs()
		}
	case "enter", "l", "right":
		if _, ok := m.currentSlice(); ok {
			return m, m.enterCockpit()
		}
	case "a":
		return m, m.attachCmd()
	}
	return m, nil
}

// snapFocusToFilter ensures m.focus points at a slice matching the filter.
func (m *Model) snapFocusToFilter() {
	idxs := m.filteredIndices()
	if len(idxs) == 0 {
		return
	}
	if posInFiltered(idxs, m.focus) < 0 {
		m.focus = idxs[0]
	}
}

// loadFocusedPRs lazily loads PR data for the focused slice (used to fill the
// browser's PR badge and overview title without a gh storm across all slices).
func (m *Model) loadFocusedPRs() tea.Cmd {
	return m.maybeLoadPRs()
}

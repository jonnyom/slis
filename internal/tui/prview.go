package tui

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/safeterm"
)

var (
	htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`) // <!-- linear linkback etc -->
	htmlTagRe     = regexp.MustCompile(`<[^>]+>`)        // <a …>, <img …>, …
	mdImageRe     = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLinkRe      = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`) // keep the link text
)

// cleanCommentBody turns a raw PR comment body into a single readable line. Bot
// comments (Graphite, Linear, CI) embed HTML and markdown that otherwise render
// as garbage. It decodes HTML entities (&gt; → >, &#39; → ') BEFORE stripping
// tags (so entity-encoded tags like &lt;img&gt; are removed too), drops HTML
// comments/tags and markdown image/link syntax, removes terminal escapes, and
// collapses whitespace. The overlay wraps the result to the pane width.
func cleanCommentBody(s string) string {
	s = htmlCommentRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = mdImageRe.ReplaceAllString(s, "")
	s = mdLinkRe.ReplaceAllString(s, "$1") // keep link text, drop the URL
	s = htmlTagRe.ReplaceAllString(s, "")
	s = safeterm.Strip(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// wrapText word-wraps s to width columns (byte-approximate; fine for the mostly
// ASCII prose of PR comments). Returns at least one line.
func wrapText(s string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
		} else {
			cur += " " + w
		}
	}
	return append(lines, cur)
}

// commentsWidth is the wrap width for the comments overlay, derived from the
// terminal width with sensible bounds.
func commentsWidth(m Model) int {
	w := m.width - 10
	if w > 100 {
		w = 100
	}
	if w < 40 {
		w = 40
	}
	return w
}

// prsLoadedMsg is sent when PR data for a slice has been loaded off the UI goroutine.
type prsLoadedMsg struct {
	slice string
	prs   map[string]*forge.PR // repo name → PR (nil means checked, no PR found)
}

// loadPRsCmd returns a Cmd that loads PR data for all members of a slice off
// the UI goroutine. Errors are swallowed per-repo — a nil *forge.PR entry
// means no PR was found (or gh is absent / failed).
func loadPRsCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		prs := make(map[string]*forge.PR, len(sl.Members))
		for repo, member := range sl.Members {
			pr, _ := forge.PRForBranch(member.WorktreePath, member.Branch)
			prs[repo] = pr
		}
		return prsLoadedMsg{slice: sl.Name, prs: prs}
	}
}

// maybeLoadPRs returns a loadPRsCmd for the focused slice if its PR data is not
// already cached or being loaded. Returns nil if no load is needed.
func (m *Model) maybeLoadPRs() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	if _, cached := m.prs[sl.Name]; cached {
		return nil
	}
	if m.prLoading[sl.Name] {
		return nil
	}
	m.prLoading[sl.Name] = true
	return loadPRsCmd(sl)
}

// copyToClipboardCmd returns a tea.Cmd that writes text to the system clipboard.
// Uses pbcopy on darwin and xclip/xsel on linux. Best-effort — errors are ignored.
func copyToClipboardCmd(text string) tea.Cmd {
	name, args, ok := clipboardCmd()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		c := exec.Command(name, args...) //nolint:gosec
		c.Stdin = strings.NewReader(text)
		_ = c.Run() // best-effort; ignore errors
		return nil
	}
}

// copyPRStackCmd builds and copies a PR stack markdown for the focused slice.
// If PRs are not yet loaded, it returns nil (caller should ensure PRs are loaded first).
func copyPRStackCmd(m Model) tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	slicePRs, ok := m.prs[sl.Name]
	if !ok {
		return nil
	}
	repos := sl.Repos()
	prs := make([]*forge.PR, 0, len(repos))
	for _, repo := range repos {
		if pr := slicePRs[repo]; pr != nil {
			prs = append(prs, pr)
		}
	}
	if len(prs) == 0 {
		return nil
	}
	md := forge.StackMarkdown(sl.Name, prs)
	return copyToClipboardCmd(md)
}

// fixCICmd returns a tea.Cmd that hands the terminal to `slis fix-ci <slice>`
// via tea.ExecProcess. Uses os.Executable to locate the running binary.
// Returns nil if the executable cannot be determined.
func fixCICmd(sl model.Slice) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "fix-ci", sl.Name) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return nil })
}

// focusedPR returns the PR (and its repo) the cockpit's focused panel points at:
// the PRs panel uses prSel, other panels use repoSel.
func (m Model) focusedPR(sl model.Slice) (*forge.PR, string) {
	repos := sl.Repos()
	if len(repos) == 0 {
		return nil, ""
	}
	sel := m.repoSel
	if m.panel == panelPRs {
		sel = m.prSel
	}
	repo := repos[clamp(sel, 0, len(repos)-1)]
	if prs := m.prs[sl.Name]; prs != nil {
		return prs[repo], repo
	}
	return nil, repo
}

// ciRerunMsg reports the result of a `gh run rerun` action.
type ciRerunMsg struct {
	n   int
	err error
}

// rerunCICmd re-triggers the failed CI runs for pr (off the UI goroutine).
func rerunCICmd(worktree string, pr *forge.PR) tea.Cmd {
	return func() tea.Msg {
		n, err := forge.RerunFailedChecks(worktree, pr)
		return ciRerunMsg{n: n, err: err}
	}
}

// ciLogLoadedMsg carries the fetched failed-CI log (or an error).
type ciLogLoadedMsg struct {
	log string
	err error
}

// loadCILogCmd fetches the failed-step logs for pr (off the UI goroutine).
func loadCILogCmd(worktree string, pr *forge.PR) tea.Cmd {
	return func() tea.Msg {
		out, err := forge.FailedLog(worktree, pr)
		if err != nil {
			return ciLogLoadedMsg{err: err}
		}
		return ciLogLoadedMsg{log: safeterm.StripNonSGR(out)}
	}
}

// ciLogContent renders the right pane in rightCILog mode.
func ciLogContent(m Model) string {
	if m.ciLogLoading {
		return "loading CI logs…"
	}
	if strings.TrimSpace(m.ciLog) == "" {
		return "no CI logs — press [v] on a PR with failing checks"
	}
	return m.ciLog
}

// commentLine flattens a single comment from a PR into a clean displayable line.
// commentSummaryLine renders a comment as one compact line — repo #N author:
// cleaned body, truncated — for the PR-detail pane's list.
func commentSummaryLine(repo string, pr *forge.PR, c forge.Comment) string {
	author := c.Author
	if author == "" {
		author = "?"
	}
	body := cleanCommentBody(c.Body)
	if body == "" {
		body = "(no text)"
	}
	if r := []rune(body); len(r) > 100 {
		body = string(r[:99]) + "…"
	}
	return fmt.Sprintf("%s #%d %s: %s", repo, pr.Number, author, body)
}

// commentBlock renders one comment as a header line, its cleaned body wrapped to
// width, then a trailing blank line as a separator.
func commentBlock(repo string, pr *forge.PR, c forge.Comment, width int) []string {
	author := c.Author
	if author == "" {
		author = "?"
	}
	lines := []string{commentHeaderStyle.Render(fmt.Sprintf("%s #%d — %s", repo, pr.Number, author))}

	body := cleanCommentBody(c.Body)
	if body == "" {
		body = "(no text)"
	}
	lines = append(lines, wrapText(body, width)...)
	return append(lines, "")
}

// flattenComments returns the rendered comment lines across the focused slice's
// PRs, in repo order (header + wrapped body + separator per comment).
func flattenComments(m Model) []string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	slicePRs := m.prs[sl.Name]
	if slicePRs == nil {
		return nil
	}
	width := commentsWidth(m)
	repos := sl.Repos()
	var lines []string
	for _, repo := range repos {
		pr := slicePRs[repo]
		if pr == nil {
			continue
		}
		for _, c := range pr.Comments {
			lines = append(lines, commentBlock(repo, pr, c, width)...)
		}
	}
	return lines
}

var (
	commentsOverlayBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1).
				BorderForeground(lipgloss.Color("99"))
	commentsOverlayNormStyle = lipgloss.NewStyle()
	commentHeaderStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
)

// commentsWindow is how many rendered comment lines are shown at once.
const commentsWindow = 22

// clampCommentScroll bounds the scroll offset so the window never runs past the
// end of the rendered lines.
func clampCommentScroll(sel, total int) int {
	maxStart := total - commentsWindow
	if maxStart < 0 {
		maxStart = 0
	}
	if sel < 0 {
		return 0
	}
	if sel > maxStart {
		return maxStart
	}
	return sel
}

// renderCommentsOverlay renders the comments overlay for the focused slice.
// m.commentsSel is the scroll offset (top visible line), not a selection.
func renderCommentsOverlay(m Model) string {
	var sb strings.Builder

	sliceName := ""
	if len(m.slices) > 0 && m.focus >= 0 && m.focus < len(m.slices) {
		sliceName = m.slices[m.focus].Name
	}

	title := fmt.Sprintf("Comments — %s — [j/k] scroll  [c/esc] close", sliceName)
	sb.WriteString(title)
	sb.WriteString("\n\n")

	lines := flattenComments(m)
	if len(lines) == 0 {
		sb.WriteString("(no comments)\n")
		return commentsOverlayBoxStyle.Render(sb.String())
	}

	total := len(lines)
	start := clampCommentScroll(m.commentsSel, total)
	end := start + commentsWindow
	if end > total {
		end = total
	}

	if start > 0 {
		sb.WriteString(cockpitDimStyle.Render("    ↑ more above") + "\n")
	}
	for i := start; i < end; i++ {
		sb.WriteString(commentsOverlayNormStyle.Render(lines[i]))
		sb.WriteString("\n")
	}
	if end < total {
		sb.WriteString(cockpitDimStyle.Render("    ↓ more below"))
	}

	return commentsOverlayBoxStyle.Render(sb.String())
}

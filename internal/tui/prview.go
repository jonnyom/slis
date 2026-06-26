package tui

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/commentcache"
	"github.com/jonnyom/slis/internal/config"
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

// prsLoadedMsg is sent when PR data for a slice has been loaded off the UI goroutine.
type prsLoadedMsg struct {
	slice string
	prs   map[string]*forge.PR // repo name → PR (nil means checked, no PR found)
}

// loadPRsCmd returns a Cmd that loads PR data for all members of a slice off
// the UI goroutine. Errors are swallowed per-repo — a nil *forge.PR entry
// means no PR was found (or gh is absent / failed).
func loadPRsCmd(sl model.Slice) tea.Cmd {
	return gatedCmd(func() tea.Msg {
		prs := make(map[string]*forge.PR, len(sl.Members))
		for repo, member := range sl.Members {
			pr, _ := forge.PRForBranch(member.WorktreePath, member.Branch)
			prs[repo] = pr
		}
		return prsLoadedMsg{slice: sl.Name, prs: prs}
	})
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

// forceLoadPRs reloads the focused slice's PRs/comments unconditionally, ignoring
// the cache — used when opening the comments overlay so it always shows fresh
// data ("reload on each open").
func (m *Model) forceLoadPRs() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	m.prLoading[sl.Name] = true
	return loadPRsCmd(sl)
}

// commentCacheMsg carries the (re)loaded persistent comment cache.
type commentCacheMsg struct{ store commentcache.Store }

// loadCommentCacheCmd reads the persisted comment cache off the UI loop.
func loadCommentCacheCmd() tea.Cmd {
	return func() tea.Msg {
		store, _ := commentcache.Load(config.StatePaths().Comments)
		return commentCacheMsg{store: store}
	}
}

// persistCommentsCmd writes a slice's freshly-fetched comments into the on-disk
// cache (so they survive the slice being cleared) and returns the updated store
// so the in-memory copy stays current. Empty/absent PRs are skipped, never
// clobbering previously-cached comments.
func persistCommentsCmd(slice string, prs map[string]*forge.PR) tea.Cmd {
	return func() tea.Msg {
		path := config.StatePaths().Comments
		store, _ := commentcache.Load(path)
		for repo, pr := range prs {
			if pr == nil || len(pr.Comments) == 0 {
				continue
			}
			cs := make([]commentcache.Comment, 0, len(pr.Comments))
			for _, c := range pr.Comments {
				cs = append(cs, commentcache.Comment{
					Author:  c.Author,
					Body:    c.Body,
					URL:     c.URL,
					Kind:    int(c.Kind),
					Context: c.Context,
				})
			}
			store.Put(slice, repo, pr.Number, pr.URL, cs)
		}
		_ = store.Save(path)
		return commentCacheMsg{store: store}
	}
}

// prsTickMsg drives periodic refresh of the focused slice's PRs/comments.
type prsTickMsg struct{}

// prsTickCmd schedules the next PR/comments refresh.
func prsTickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return prsTickMsg{} })
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

var (
	approvedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))  // approved (green)
	changesStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // changes requested (red)
)

// reviewBadge renders a PR's review decision as a compact coloured badge.
// Returns "" when there is no decision (so callers can append unconditionally).
func reviewBadge(decision string) string {
	switch strings.ToUpper(decision) {
	case "APPROVED":
		return approvedStyle.Render("✓ approved")
	case "CHANGES_REQUESTED":
		return changesStyle.Render("✗ changes")
	case "REVIEW_REQUIRED":
		return cockpitDimStyle.Render("· review")
	}
	return ""
}

// ciBadge renders a PR's CI rollup as an emoji plus a failing-check count when
// any check is failing (e.g. "❌2"), for the compact hub rows.
func ciBadge(pr *forge.PR) string {
	overall, _, fail, _ := pr.CISummary()
	s := forge.CIEmoji(overall)
	if fail > 0 {
		s += fmt.Sprintf("%d", fail)
	}
	return s
}

var commentHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))

// reviewStateLabel renders a PR review's state as a short plain-text label for a
// comment header (the header is styled as a whole, so this stays unstyled).
func reviewStateLabel(state string) string {
	switch strings.ToUpper(state) {
	case "APPROVED":
		return "✓ approved"
	case "CHANGES_REQUESTED":
		return "✗ changes"
	case "COMMENTED":
		return "💬 review"
	case "DISMISSED":
		return "dismissed"
	}
	return "review"
}

// commentKindLabel marks a comment's origin: 💬 for an issue comment, the review
// state for a review submission, and "📝 path:line" for an inline review comment.
func commentKindLabel(c forge.Comment) string {
	switch c.Kind {
	case forge.CommentReview:
		return reviewStateLabel(c.Context)
	case forge.CommentInline:
		if c.Context != "" {
			return "📝 " + c.Context
		}
		return "📝 inline"
	default:
		return "💬"
	}
}

// commentBlock renders one comment as a header line (kind label · repo #N ·
// author), its cleaned body wrapped to width, then a trailing blank separator.
func commentBlock(repo string, pr *forge.PR, c forge.Comment, width int) []string {
	author := c.Author
	if author == "" {
		author = "?"
	}
	header := fmt.Sprintf("%s  %s #%d — %s", commentKindLabel(c), repo, pr.Number, author)
	lines := []string{commentHeaderStyle.Render(header)}

	body := cleanCommentBody(c.Body)
	if body == "" {
		body = "(no text)"
	}
	lines = append(lines, wrapText(body, width)...)
	return append(lines, "")
}

// repoCommentBlocks renders the comment blocks for one repo's PR: live comments
// when the PR is loaded, else the persisted cache (so comments survive the
// branch/worktree being cleared). Returns nil when there are none.
func repoCommentBlocks(m Model, slice, repo string, pr *forge.PR, width int) []string {
	if pr != nil {
		var lines []string
		for _, c := range pr.Comments {
			lines = append(lines, commentBlock(repo, pr, c, width)...)
		}
		return lines
	}
	rc, ok := m.commentCache[slice][repo]
	if !ok {
		return nil
	}
	cpr := &forge.PR{Number: rc.PR, URL: rc.URL}
	var lines []string
	for _, c := range rc.Comments {
		lines = append(lines, commentBlock(repo, cpr, forge.Comment{
			Author:  c.Author,
			Body:    c.Body,
			URL:     c.URL,
			Kind:    forge.CommentKind(c.Kind),
			Context: c.Context,
		}, width)...)
	}
	return lines
}

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
)

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

// commentLine flattens a single comment from a PR into a displayable line.
func commentLine(repo string, pr *forge.PR, c forge.Comment) string {
	firstLine := c.Body
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	return fmt.Sprintf("%s #%d %s: %s", repo, pr.Number, c.Author, firstLine)
}

// flattenComments returns all comment lines across the focused slice's PRs, in repo order.
func flattenComments(m Model) []string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	slicePRs := m.prs[sl.Name]
	if slicePRs == nil {
		return nil
	}
	repos := sl.Repos()
	var lines []string
	for _, repo := range repos {
		pr := slicePRs[repo]
		if pr == nil {
			continue
		}
		for _, c := range pr.Comments {
			lines = append(lines, commentLine(repo, pr, c))
		}
	}
	return lines
}

var (
	commentsOverlayBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1).
				BorderForeground(lipgloss.Color("99"))
	commentsOverlaySelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Reverse(true)
	commentsOverlayNormStyle = lipgloss.NewStyle()
)

// renderCommentsOverlay renders the comments overlay for the focused slice.
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

	const windowSize = 20
	total := len(lines)
	sel := m.commentsSel
	if sel < 0 {
		sel = 0
	}
	if sel >= total {
		sel = total - 1
	}

	start := sel - windowSize/2
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > total {
		end = total
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		line := lines[i]
		if i == sel {
			sb.WriteString(commentsOverlaySelStyle.Render(line))
		} else {
			sb.WriteString(commentsOverlayNormStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	return commentsOverlayBoxStyle.Render(sb.String())
}

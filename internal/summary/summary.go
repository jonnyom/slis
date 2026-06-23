// Package summary provides commit-log aggregation and AI-assisted prose summaries for slices.
package summary

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
)

// aiSummaryTimeout bounds the AI summary call so a hung `claude` process can't
// leave the TUI stuck on "generating summary…" forever.
const aiSummaryTimeout = 120 * time.Second

// CommitSummary returns, per repo (sorted), the commit subjects on the slice
// branch since base (git log --format=%s base..HEAD in the member worktree).
// A repo whose base ref is missing yields an empty list (no error for that repo).
//
// base is the ref to log against. Pass "" to auto-detect each repo's trunk
// independently (git.DetectBase); a non-empty base is used verbatim for all.
func CommitSummary(sl model.Slice, base string) (map[string][]string, error) {
	bases := make(map[string]string, len(sl.Members))
	for repo := range sl.Members {
		bases[repo] = base
	}
	return CommitSummaryBases(sl, bases)
}

// CommitSummaryBases is like CommitSummary but takes a per-repo base (bases[repo]).
// A repo with no entry (or "") auto-detects its trunk. Used to log a stacked
// branch against its Graphite parent so the count reflects only that branch.
func CommitSummaryBases(sl model.Slice, bases map[string]string) (map[string][]string, error) {
	result := make(map[string][]string, len(sl.Members))
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		b := bases[repo]
		if b == "" {
			b = git.DetectBase(m.WorktreePath)
		}
		out, err := git.Run(m.WorktreePath, "log", "--format=%s", b+"..HEAD")
		if err != nil {
			// base ref may be absent — treat as no commits for this repo.
			result[repo] = []string{}
			continue
		}
		var subjects []string
		for _, line := range strings.Split(out, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				subjects = append(subjects, s)
			}
		}
		result[repo] = subjects
	}
	return result, nil
}

// RenderCommitSummary formats the per-repo commit subjects as markdown.
// Repos are output in sorted order. Empty repos get a "(no commits)" note.
func RenderCommitSummary(byRepo map[string][]string) string {
	// Sort repos for stable output.
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)

	var sb strings.Builder
	for _, repo := range repos {
		fmt.Fprintf(&sb, "## %s\n\n", repo)
		subjects := byRepo[repo]
		if len(subjects) == 0 {
			sb.WriteString("- (no commits)\n")
		} else {
			for _, s := range subjects {
				fmt.Fprintf(&sb, "- %s\n", s)
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// AISummary builds an instruction prompt and calls runner(instruction, diff) to
// produce a prose summary of the combined diff. runner is injected for testing;
// the default (DefaultClaudeRunner) shells out to `claude -p <instruction>` with
// the diff piped on stdin.
func AISummary(combinedDiff string, runner func(instruction, stdin string) (string, error)) (string, error) {
	instruction := "Summarise the following multi-repo diff for a teammate reviewing this feature. Be concise: what changed and why, grouped by area. Output markdown."
	return runner(instruction, combinedDiff)
}

// DefaultClaudeRunner runs `claude -p <instruction>` with stdin=content. Returns
// an error (with a clear message) if the `claude` binary is not on PATH. The
// call is bounded by aiSummaryTimeout so a hung process surfaces as an error
// rather than a permanent "generating summary…" state.
func DefaultClaudeRunner(instruction, content string) (string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return "", errors.New("claude CLI not found: install claude and ensure it is on your PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiSummaryTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "claude", "-p", instruction) //nolint:gosec
	c.Stdin = strings.NewReader(content)
	out, err := c.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("claude -p timed out after %s", aiSummaryTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("claude -p: %w", err)
	}
	return string(out), nil
}

// RenderMarkdown renders markdown to styled terminal text via glamour; on any
// glamour error it returns the input unchanged (never fails the caller).
func RenderMarkdown(md string) string {
	r, err := glamour.NewTermRenderer(glamour.WithAutoStyle())
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

// RenderMarkdownFixed renders markdown with a FIXED dark style and word-wrap,
// performing NO terminal background-color query. Safe to call from inside a
// running Bubble Tea program (unlike RenderMarkdown, which uses WithAutoStyle
// and would block on a terminal query there). Returns md unchanged on any error.
func RenderMarkdownFixed(md string, wrap int) string {
	if wrap <= 0 {
		wrap = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

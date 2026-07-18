package review

import (
	"fmt"
	"strings"
)

// ComposePrompt turns a batch of review comments into a single, structured
// instruction block for the slice's agent. Comments are emitted in deterministic
// order (repo, file, line); each item names the repo and file:line, quotes the
// hunk as fenced context when present, and states the reviewer's instruction.
// An empty batch yields an empty string.
func ComposePrompt(comments []Comment) string {
	if len(comments) == 0 {
		return ""
	}

	ordered := make([]Comment, len(comments))
	copy(ordered, comments)
	sortComments(ordered)

	var b strings.Builder
	fmt.Fprintf(&b, "Code review feedback on slice %s — address each item:\n", ordered[0].Slice)

	for i, c := range ordered {
		b.WriteString("\n")
		line := fmt.Sprintf("%d", c.Line)
		if c.EndLine > c.Line {
			line = fmt.Sprintf("%d-%d", c.Line, c.EndLine)
		}
		location := fmt.Sprintf("%s — %s:%s", c.Repo, c.File, line)
		if c.Side == "old" {
			location += " (old/deleted side)"
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, location)

		if hunk := strings.TrimRight(c.Hunk, "\n"); hunk != "" {
			b.WriteString("```\n")
			b.WriteString(hunk)
			b.WriteString("\n```\n")
		}

		fmt.Fprintf(&b, "%s\n", strings.TrimRight(c.Body, "\n"))
	}

	return b.String()
}

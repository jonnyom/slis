package review

import (
	"strings"
	"testing"
)

func TestComposePromptEmpty(t *testing.T) {
	if got := ComposePrompt(nil); got != "" {
		t.Errorf("ComposePrompt(nil) = %q, want empty", got)
	}
}

func TestComposePromptStructureAndOrder(t *testing.T) {
	// Deliberately unordered input; ComposePrompt must sort (repo, file, line).
	comments := []Comment{
		{Slice: "checkout", Repo: "web", File: "pay.go", Line: 42, Body: "rename this variable"},
		{Slice: "checkout", Repo: "api", File: "handler.go", Line: 7,
			Hunk: "func Handle() {\n\treturn nil\n}", Body: "return the error"},
	}

	want := "Code review feedback on slice checkout — address each item:\n" +
		"\n1. api — handler.go:7\n" +
		"```\n" +
		"func Handle() {\n\treturn nil\n}\n" +
		"```\n" +
		"return the error\n" +
		"\n2. web — pay.go:42\n" +
		"rename this variable\n"

	if got := ComposePrompt(comments); got != want {
		t.Errorf("ComposePrompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestComposePromptOmitsEmptyHunk(t *testing.T) {
	got := ComposePrompt([]Comment{{Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "do x"}})
	want := "Code review feedback on slice s — address each item:\n" +
		"\n1. web — a.go:1\n" +
		"do x\n"
	if got != want {
		t.Errorf("ComposePrompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestComposePromptIncludesLineRange(t *testing.T) {
	got := ComposePrompt([]Comment{{
		Slice: "s", Repo: "web", File: "a.go", Line: 4, EndLine: 7, Body: "extract this block",
	}})
	want := "Code review feedback on slice s — address each item:\n" +
		"\n1. web — a.go:4-7\n" +
		"extract this block\n"
	if got != want {
		t.Errorf("ComposePrompt mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestComposePromptIdentifiesOldSide(t *testing.T) {
	got := ComposePrompt([]Comment{{
		Slice: "s", Repo: "web", File: "a.go", Line: 9, Side: "old", Body: "keep this behavior",
	}})
	if !strings.Contains(got, "web — a.go:9 (old/deleted side)") {
		t.Fatalf("old-side prompt location missing:\n%s", got)
	}
}

func TestComposePromptIdentifiesReviewer(t *testing.T) {
	got := ComposePrompt([]Comment{{
		Slice: "s", Repo: "web", File: "a.go", Line: 9, Side: "new", Body: "guard this write", Author: "Codex",
	}})
	if !strings.Contains(got, "web — a.go:9 — reviewer: Codex") {
		t.Fatalf("reviewer missing from prompt location:\n%s", got)
	}
}

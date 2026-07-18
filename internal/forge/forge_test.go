package forge_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/forge"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/pr.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestParsePR(t *testing.T) {
	pr, err := forge.ParsePR("demo/pr-features", fixture(t))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	if pr == nil {
		t.Fatal("expected non-nil PR")
	}
	if pr.Number != 1 {
		t.Errorf("Number: got %d, want 1", pr.Number)
	}
	if pr.URL != "https://github.com/jonnyom/slis/pull/1" {
		t.Errorf("URL: got %q", pr.URL)
	}
	if pr.State != "OPEN" {
		t.Errorf("State: got %q, want OPEN", pr.State)
	}
	if pr.Title != "demo: PR features test" {
		t.Errorf("Title: got %q", pr.Title)
	}
	if pr.Branch != "demo/pr-features" {
		t.Errorf("Branch: got %q, want demo/pr-features", pr.Branch)
	}
	if len(pr.Checks) != 3 {
		t.Errorf("len(Checks): got %d, want 3", len(pr.Checks))
	}
	if len(pr.Comments) != 1 {
		t.Errorf("len(Comments): got %d, want 1", len(pr.Comments))
	}
	if pr.Comments[0].Author != "jonnyom" {
		t.Errorf("Comments[0].Author: got %q, want jonnyom", pr.Comments[0].Author)
	}
}

func TestParsePRPreservesHeadSHA(t *testing.T) {
	raw := `{"number":151,"state":"MERGED","headRefName":"test","headRefOid":"8b14a2fedf02b1cc73e0282b6c6caf0d86d4dd23"}`
	pr, err := forge.ParsePR("test", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if pr.HeadSHA != "8b14a2fedf02b1cc73e0282b6c6caf0d86d4dd23" {
		t.Fatalf("HeadSHA = %q", pr.HeadSHA)
	}
}

func TestCISummary(t *testing.T) {
	pr, err := forge.ParsePR("demo/pr-features", fixture(t))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	overall, pass, fail, pending := pr.CISummary()
	if overall != forge.CheckFail {
		t.Errorf("overall: got %v, want CheckFail", overall)
	}
	if pass != 1 {
		t.Errorf("pass: got %d, want 1", pass)
	}
	if fail != 1 {
		t.Errorf("fail: got %d, want 1", fail)
	}
	if pending != 1 {
		t.Errorf("pending: got %d, want 1", pending)
	}
}

func TestFailingChecks(t *testing.T) {
	pr, err := forge.ParsePR("demo/pr-features", fixture(t))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	failing := pr.FailingChecks()
	if len(failing) != 1 {
		t.Fatalf("len(FailingChecks): got %d, want 1", len(failing))
	}
	if failing[0].Name != "build / security-scan (ubuntu-latest)" {
		t.Errorf("failing[0].Name: got %q", failing[0].Name)
	}
	if failing[0].State != forge.CheckFail {
		t.Errorf("failing[0].State: got %v, want CheckFail", failing[0].State)
	}
}

func TestCheckStateNormalization(t *testing.T) {
	raw := `{
		"number": 2,
		"url": "https://github.com/jonnyom/slis/pull/2",
		"state": "OPEN",
		"title": "legacy ci test",
		"headRefName": "legacy/ci",
		"reviewDecision": "",
		"statusCheckRollup": [
			{"__typename":"StatusContext","context":"ci/legacy","state":"SUCCESS","targetUrl":"https://legacy.ci/job/42"}
		],
		"comments": []
	}`
	pr, err := forge.ParsePR("legacy/ci", []byte(raw))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	if len(pr.Checks) != 1 {
		t.Fatalf("len(Checks): got %d, want 1", len(pr.Checks))
	}
	c := pr.Checks[0]
	if c.Name != "ci/legacy" {
		t.Errorf("Name: got %q, want ci/legacy", c.Name)
	}
	if c.State != forge.CheckPass {
		t.Errorf("State: got %v, want CheckPass", c.State)
	}
	if c.URL != "https://legacy.ci/job/42" {
		t.Errorf("URL: got %q, want https://legacy.ci/job/42", c.URL)
	}
}

func TestParsePRNoChecks(t *testing.T) {
	raw := `{
		"number": 3,
		"url": "https://github.com/jonnyom/slis/pull/3",
		"state": "OPEN",
		"title": "no checks",
		"headRefName": "no-checks",
		"reviewDecision": "",
		"statusCheckRollup": [],
		"comments": []
	}`
	pr, err := forge.ParsePR("no-checks", []byte(raw))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	overall, pass, fail, pending := pr.CISummary()
	if overall != forge.CheckPending {
		t.Errorf("overall: got %v, want CheckPending", overall)
	}
	if pass != 0 || fail != 0 || pending != 0 {
		t.Errorf("expected all-zero counts, got pass=%d fail=%d pending=%d", pass, fail, pending)
	}
}

// TestParsePRNullChecks ensures a nil statusCheckRollup also gives Pending.
func TestParsePRNullChecks(t *testing.T) {
	raw := `{
		"number": 4,
		"url": "https://github.com/jonnyom/slis/pull/4",
		"state": "OPEN",
		"title": "null checks",
		"headRefName": "null-checks",
		"reviewDecision": "",
		"statusCheckRollup": null,
		"comments": null
	}`
	pr, err := forge.ParsePR("null-checks", []byte(raw))
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	overall, _, _, _ := pr.CISummary()
	if overall != forge.CheckPending {
		t.Errorf("overall: got %v, want CheckPending", overall)
	}
}

// TestParsePRBadJSON ensures ParsePR returns an error on malformed input.
func TestParsePRBadJSON(t *testing.T) {
	_, err := forge.ParsePR("branch", []byte("not json"))
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
}

// TestAvailable checks that Available() returns a bool without panicking.
func TestAvailable(t *testing.T) {
	_ = forge.Available()
}

// TestCIEmoji verifies the emoji helper for each CheckState.
func TestCIEmoji(t *testing.T) {
	tests := []struct {
		state forge.CheckState
		want  string
	}{
		{forge.CheckPass, "✅"},
		{forge.CheckFail, "❌"},
		{forge.CheckPending, "⏳"},
	}
	for _, tc := range tests {
		got := forge.CIEmoji(tc.state)
		if got != tc.want {
			t.Errorf("CIEmoji(%v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// TestStackMarkdown verifies StackMarkdown renders correct output for a set of PRs.
func TestStackMarkdown(t *testing.T) {
	pr1 := &forge.PR{
		Branch: "feat/api-changes",
		Number: 42,
		URL:    "https://github.com/acme/api/pull/42",
		Title:  "feat: add new API endpoint",
		Checks: []forge.Check{
			{Name: "ci/build", State: forge.CheckFail},
			{Name: "ci/lint", State: forge.CheckPass},
		},
		Comments: []forge.Comment{
			{Author: "alice", Body: "looks good"},
			{Author: "bob", Body: "nit: rename variable"},
		},
	}
	pr2 := &forge.PR{
		Branch: "feat/web-update",
		Number: 99,
		URL:    "https://github.com/acme/web/pull/99",
		Title:  "chore: update dependencies",
		Checks: []forge.Check{
			{Name: "ci/build", State: forge.CheckPass},
		},
		Comments: nil,
	}

	md := forge.StackMarkdown("my-feature", []*forge.PR{pr1, pr2})

	// Must contain the title
	if !strings.Contains(md, "my-feature") {
		t.Error("markdown missing stack title")
	}
	// PR numbers present
	if !strings.Contains(md, "#42") {
		t.Error("markdown missing #42")
	}
	if !strings.Contains(md, "#99") {
		t.Error("markdown missing #99")
	}
	// URLs present
	if !strings.Contains(md, "https://github.com/acme/api/pull/42") {
		t.Error("markdown missing URL for #42")
	}
	if !strings.Contains(md, "https://github.com/acme/web/pull/99") {
		t.Error("markdown missing URL for #99")
	}
	// CI emoji: failing PR gets ❌, passing gets ✅
	if !strings.Contains(md, "❌") {
		t.Error("markdown missing ❌ for failing CI")
	}
	if !strings.Contains(md, "✅") {
		t.Error("markdown missing ✅ for passing CI")
	}
	// Comments count for pr1
	if !strings.Contains(md, "💬 2") {
		t.Error("markdown missing '💬 2' comment indicator")
	}
}

// TestStackMarkdownNilSkipped verifies that nil PR entries are silently skipped.
func TestStackMarkdownNilSkipped(t *testing.T) {
	pr := &forge.PR{
		Branch: "feat/x",
		Number: 1,
		URL:    "https://github.com/acme/x/pull/1",
		Title:  "feat: x",
	}
	md := forge.StackMarkdown("title", []*forge.PR{nil, pr, nil})
	if !strings.Contains(md, "#1") {
		t.Error("markdown missing #1 when surrounded by nil entries")
	}
}

// TestParsePRReviews verifies that bodied review submissions become comments
// (tagged CommentReview with the review state as Context) while bare approvals
// and pending/dismissed reviews are skipped.
func TestParsePRReviews(t *testing.T) {
	data, err := os.ReadFile("testdata/pr_reviews.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	pr, err := forge.ParsePR("feature/punch", data)
	if err != nil {
		t.Fatalf("ParsePR error: %v", err)
	}
	// 1 issue comment + mahesh (approved, bodied) + reviewer (changes_requested).
	// someone (bare approval) and draft (pending) are skipped.
	if len(pr.Comments) != 3 {
		t.Fatalf("len(Comments): got %d, want 3 (%+v)", len(pr.Comments), pr.Comments)
	}

	var sawMahesh, sawChanges bool
	for _, c := range pr.Comments {
		if c.Author == "someone" {
			t.Errorf("bare approval (no body) should be skipped, but was included: %+v", c)
		}
		if c.Author == "mahesh" {
			sawMahesh = true
			if c.Kind != forge.CommentReview {
				t.Errorf("mahesh review Kind: got %v, want CommentReview", c.Kind)
			}
			if c.Context != "approved" {
				t.Errorf("mahesh review Context: got %q, want approved", c.Context)
			}
			if !strings.Contains(c.Body, "Approving it") {
				t.Errorf("mahesh review body: got %q", c.Body)
			}
		}
		if c.Context == "changes_requested" {
			sawChanges = true
			if c.Kind != forge.CommentReview {
				t.Errorf("changes-requested Kind: got %v, want CommentReview", c.Kind)
			}
		}
	}
	if !sawMahesh {
		t.Error("expected mahesh's bodied approval as a review comment")
	}
	if !sawChanges {
		t.Error("expected a changes_requested review comment")
	}
}

// TestParseInlineComments verifies inline review comments parse into
// CommentInline entries with a "path:line" Context, falling back to
// original_line when line is 0.
func TestParseInlineComments(t *testing.T) {
	data, err := os.ReadFile("testdata/comments_inline.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cs, err := forge.ParseInlineComments(data)
	if err != nil {
		t.Fatalf("ParseInlineComments error: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("len: got %d, want 2", len(cs))
	}
	if cs[0].Kind != forge.CommentInline {
		t.Errorf("Kind: got %v, want CommentInline", cs[0].Kind)
	}
	if cs[0].Context != "src/foo.go:42" {
		t.Errorf("Context[0]: got %q, want src/foo.go:42", cs[0].Context)
	}
	if cs[0].Author != "cubic" {
		t.Errorf("Author[0]: got %q, want cubic", cs[0].Author)
	}
	// line == 0 → fall back to original_line (7).
	if cs[1].Context != "src/bar.go:7" {
		t.Errorf("Context[1]: got %q, want src/bar.go:7 (original_line fallback)", cs[1].Context)
	}
}

// TestParseInlineCommentsEmpty verifies empty/blank input yields no comments,
// no error.
func TestParseInlineCommentsEmpty(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("  "), []byte("[]")} {
		cs, err := forge.ParseInlineComments(in)
		if err != nil {
			t.Errorf("ParseInlineComments(%q) error: %v", in, err)
		}
		if len(cs) != 0 {
			t.Errorf("ParseInlineComments(%q): got %d comments, want 0", in, len(cs))
		}
	}
}

// TestCheckStateJSONRoundtrip ensures CheckState values are numerically stable.
func TestCheckStateJSONRoundtrip(t *testing.T) {
	states := []forge.CheckState{forge.CheckPending, forge.CheckPass, forge.CheckFail}
	for _, s := range states {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", s, err)
		}
		var got forge.CheckState
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got != s {
			t.Errorf("roundtrip: got %v, want %v", got, s)
		}
	}
}

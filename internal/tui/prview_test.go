package tui

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/commentcache"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// prTestModel returns a model pre-loaded with one slice "s" containing
// member "web" with a cached stack.
func prTestModel(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{})
	m.slices = []model.Slice{
		{
			Name: "s",
			Members: map[string]model.SliceMember{
				"web": {Repo: "web", Branch: "feature-s", WorktreePath: "/tmp/web"},
			},
		},
	}
	m.loading = false
	m.view = viewCockpit
	// Pre-populate a stack so the cockpit panels don't return "loading…"
	m.stacks = map[string]map[string]gt.State{
		"s": {
			"web": gt.State{
				"main":      gt.BranchState{Trunk: true},
				"feature-s": gt.BranchState{Parents: []gt.Parent{{Ref: "main", SHA: "abc"}}},
			},
		},
	}
	return m
}

// TestPrsLoadedMsg verifies that feeding prsLoadedMsg into Update stores PRs and
// clears the loading flag.
func TestPrsLoadedMsg(t *testing.T) {
	m := prTestModel(t)
	m.prLoading = map[string]bool{"s": true}

	pr := &forge.PR{
		Branch: "feature-s",
		Number: 5,
		URL:    "https://github.com/example/web/pull/5",
		State:  "OPEN",
	}

	next, cmd := m.Update(prsLoadedMsg{
		slice: "s",
		prs:   map[string]*forge.PR{"web": pr},
	})
	m = next.(Model)

	// prsLoadedMsg now also returns a cmd to persist comments to the cache.
	if cmd == nil {
		t.Errorf("after prsLoadedMsg: want a persist cmd, got nil")
	}

	slicePRs, ok := m.prs["s"]
	if !ok {
		t.Fatal("m.prs['s'] should be present after prsLoadedMsg")
	}
	webPR, ok := slicePRs["web"]
	if !ok {
		t.Fatal("m.prs['s']['web'] should be present after prsLoadedMsg")
	}
	if webPR.Number != 5 {
		t.Errorf("m.prs['s']['web'].Number = %d, want 5", webPR.Number)
	}
	if m.prLoading["s"] {
		t.Error("m.prLoading['s'] should be false after prsLoadedMsg")
	}
}

// TestPRsPanelWithPR verifies the cockpit PRs panel shows the PR number, a CI
// emoji, and the comment-count bubble when PRs are cached.
func TestPRsPanelWithPR(t *testing.T) {
	m := prTestModel(t)

	// PR #9, one failing check, 2 comments.
	pr := &forge.PR{
		Branch:   "feature-s",
		Number:   9,
		URL:      "https://github.com/example/web/pull/9",
		State:    "OPEN",
		Checks:   []forge.Check{{Name: "ci", State: forge.CheckFail}},
		Comments: []forge.Comment{{Author: "alice", Body: "please fix"}, {Author: "bob", Body: "agreed"}},
	}

	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": pr},
	}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "#9") {
		t.Errorf("prsPanelContent: expected '#9' in output; got:\n%s", output)
	}
	if !strings.Contains(output, "❌") {
		t.Errorf("prsPanelContent: expected fail CI emoji '❌' in output; got:\n%s", output)
	}
	if !strings.Contains(output, "💬") {
		t.Errorf("prsPanelContent: expected comment bubble '💬' in output; got:\n%s", output)
	}
}

// TestPRsPanelNoPR verifies that when PRs are loaded but a repo has none,
// "(no PR)" is shown.
func TestPRsPanelNoPR(t *testing.T) {
	m := prTestModel(t)

	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": nil},
	}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "(no PR)") {
		t.Errorf("prsPanelContent: expected '(no PR)' when PR is nil; got:\n%s", output)
	}
}

// TestPRsPanelLoading verifies that while PRs are loading the panel shows
// "PR: loading…".
func TestPRsPanelLoading(t *testing.T) {
	m := prTestModel(t)
	m.prLoading = map[string]bool{"s": true}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "PR: loading") {
		t.Errorf("prsPanelContent: expected 'PR: loading' while prLoading; got:\n%s", output)
	}
}

// TestPRsPanelReviewBadge verifies the PRs panel surfaces the review-decision badge.
func TestPRsPanelReviewBadge(t *testing.T) {
	m := prTestModel(t)
	pr := &forge.PR{
		Branch: "feature-s", Number: 9, State: "OPEN",
		ReviewDecision: "CHANGES_REQUESTED",
		Checks:         []forge.Check{{Name: "ci", State: forge.CheckFail}},
	}
	m.prs = map[string]map[string]*forge.PR{"s": {"web": pr}}

	out := prsPanelContent(m, m.slices[0])
	if !strings.Contains(out, "changes") {
		t.Errorf("prsPanelContent: expected review badge 'changes'; got:\n%s", out)
	}
}

// TestPrDetailContentComments verifies issue, review, and inline comments all
// render inline in the PR pane with their kind labels (no separate overlay).
func TestPrDetailContentComments(t *testing.T) {
	m := prTestModel(t)
	m.panel = panelPRs
	m.viewport.Width = 80

	pr := &forge.PR{
		Branch: "feature-s", Number: 3, State: "OPEN",
		ReviewDecision: "CHANGES_REQUESTED",
		Comments: []forge.Comment{
			{Author: "alice", Body: "top-level note", Kind: forge.CommentIssue},
			{Author: "mahesh", Body: "approving but fix this", Kind: forge.CommentReview, Context: "changes_requested"},
			{Author: "cubic", Body: "off-by-one here", Kind: forge.CommentInline, Context: "src/foo.go:42"},
		},
	}
	m.prs = map[string]map[string]*forge.PR{"s": {"web": pr}}

	out := prDetailContent(m, m.slices[0])
	for _, want := range []string{
		"alice", "top-level note",
		"mahesh", "approving but fix this", "✗ changes",
		"cubic", "off-by-one here", "📝 src/foo.go:42",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prDetailContent missing %q; got:\n%s", want, out)
		}
	}
}

// TestPrDetailContentCachedComments verifies that when there's no live PR, cached
// comments still render — comments survive a cleared slice.
func TestPrDetailContentCachedComments(t *testing.T) {
	m := prTestModel(t)
	m.panel = panelPRs
	m.viewport.Width = 80
	m.prs = map[string]map[string]*forge.PR{"s": {"web": nil}}
	m.commentCache = commentcache.Store{
		"s": {"web": commentcache.RepoComments{
			PR: 7, URL: "u",
			Comments: []commentcache.Comment{{Author: "ghost", Body: "from the cache"}},
		}},
	}

	out := prDetailContent(m, m.slices[0])
	if !strings.Contains(out, "ghost") || !strings.Contains(out, "from the cache") {
		t.Errorf("prDetailContent: expected cached comments to render; got:\n%s", out)
	}
}

// TestReviewBadge covers the decision→badge mapping.
func TestReviewBadge(t *testing.T) {
	cases := map[string]string{
		"APPROVED":          "approved",
		"CHANGES_REQUESTED": "changes",
		"REVIEW_REQUIRED":   "review",
		"":                  "",
	}
	for dec, want := range cases {
		got := reviewBadge(dec)
		if want == "" {
			if got != "" {
				t.Errorf("reviewBadge(%q) = %q, want empty", dec, got)
			}
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("reviewBadge(%q) = %q, want substring %q", dec, got, want)
		}
	}
}

// TestRepoDiffContentNoFlicker verifies a cached diff keeps rendering while a
// background refresh is in flight (diffLoading=true), instead of flashing "loading".
func TestRepoDiffContentNoFlicker(t *testing.T) {
	m := prTestModel(t)
	m.diffs = map[string][]diff.RepoDiff{
		"s": {{Repo: "web", Files: []diff.FileStat{{Path: "a.go", Added: 1}}}},
	}
	m.diffLoading = map[string]bool{"s": true} // refresh in flight

	out := repoDiffContent(m, m.slices[0])
	if strings.Contains(out, "loading diff") {
		t.Errorf("repoDiffContent should render the cached diff during refresh, not 'loading'; got:\n%s", out)
	}
	if !strings.Contains(out, "a.go") {
		t.Errorf("repoDiffContent: expected cached file 'a.go'; got:\n%s", out)
	}
}

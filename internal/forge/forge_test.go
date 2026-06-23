package forge_test

import (
	"encoding/json"
	"os"
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

// TestPRForBranchIntegration is guarded by Available() so it is skipped when gh
// is not on PATH or the test runner has no network.
func TestPRForBranchIntegration(t *testing.T) {
	if !forge.Available() {
		t.Skip("gh binary not available")
	}
	// We test only that calling PRForBranch on an almost-certainly-absent branch
	// returns (nil, nil) rather than an error — this exercises the no-PR path.
	repoDir := "../../" // relative to this test file; absolute not needed for exec.Command.Dir
	_, err := forge.PRForBranch(repoDir, "branch-that-does-not-exist-xyz-9999")
	if err != nil {
		t.Errorf("PRForBranch with non-existent branch should return (nil,nil), got err: %v", err)
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

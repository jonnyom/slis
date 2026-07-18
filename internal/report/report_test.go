package report

import (
	"testing"

	"github.com/jonnyom/slis/internal/forge"
)

// TestSetPRPopulatesCIRollup: SetPR copies identity/review fields and derives the
// CI rollup (state word + per-state counts) from the PR's checks.
func TestSetPRPopulatesCIRollup(t *testing.T) {
	pr := &forge.PR{
		Number:         8107,
		URL:            "https://github.com/acme/web/pull/8107",
		State:          "OPEN",
		Title:          "Checkout revamp",
		ReviewDecision: "APPROVED",
		Checks: []forge.Check{
			{Name: "build", State: forge.CheckPass},
			{Name: "lint", State: forge.CheckPass},
			{Name: "test", State: forge.CheckFail},
		},
	}
	var row PRStackRowDTO
	row.SetPR(pr)

	if row.Number != 8107 || row.State != "OPEN" || row.ReviewDecision != "APPROVED" {
		t.Fatalf("identity fields not copied: %+v", row)
	}
	if row.CI != "fail" {
		t.Errorf("ci = %q, want fail (a failing check dominates the rollup)", row.CI)
	}
	if row.CIPass != 2 || row.CIFail != 1 || row.CIPending != 0 {
		t.Errorf("counts = pass %d fail %d pending %d, want 2/1/0", row.CIPass, row.CIFail, row.CIPending)
	}
}

// TestSetPRNilLeavesBareRow: a nil PR leaves the row as a repo/branch stub with
// no CI fields, so callers can invoke it unconditionally.
func TestSetPRNilLeavesBareRow(t *testing.T) {
	row := PRStackRowDTO{Repo: "web", Branch: "jonny/checkout"}
	row.SetPR(nil)
	if row.Number != 0 || row.CI != "" || row.CIFail != 0 {
		t.Errorf("nil PR should leave a bare row, got %+v", row)
	}
}

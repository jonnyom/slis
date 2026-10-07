package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/swap"
)

func TestRecoverActivationDriftWarnsWithoutChangingCheckout(t *testing.T) {
	_, journalPath, primary := swapDoctorFixture(t)
	if _, err := git.Run(primary, "switch", "main"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(primary, "external.txt")
	if err := os.WriteFile(path, []byte("external work"), 0o644); err != nil {
		t.Fatal(err)
	}
	var warning bytes.Buffer
	if err := recoverActivationDrift(journalPath, swap.ErrUnknownPrimaryChanges, &warning); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning.String(), "deactivated") || !strings.Contains(warning.String(), "Recovery record:") {
		t.Fatalf("warning = %q", warning.String())
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "external work" {
		t.Fatalf("external work changed: %q, %v", content, err)
	}
	if branch, err := git.CurrentBranch(primary); err != nil || branch != "main" {
		t.Fatalf("branch = %q, %v", branch, err)
	}
	if active, err := swap.Load(journalPath); err != nil || active != nil {
		t.Fatalf("activation remains: %#v, %v", active, err)
	}
}

func TestRecoverActivationDriftPropagatesUnrelatedErrors(t *testing.T) {
	cause := errors.New("permission denied")
	var warning bytes.Buffer
	if err := recoverActivationDrift("unused", cause, &warning); !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	if warning.Len() != 0 {
		t.Fatalf("unexpected warning = %q", warning.String())
	}
}

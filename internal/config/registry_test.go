package config

import "testing"

func TestRegisterCreatedRecordsCreatedSourceAndMergesMembers(t *testing.T) {
	reg := Registry{}
	reg.RegisterCreated("feature", "web", "jonny/feature", "/worktrees/feature/web")
	reg.RegisterCreated("feature", "api", "jonny/feature", "/worktrees/feature/api")

	slice := reg.Slices["feature"]
	if slice.Source != SourceCreated {
		t.Fatalf("source = %q, want %q", slice.Source, SourceCreated)
	}
	if len(slice.Members) != 2 {
		t.Fatalf("members = %+v, want web + api", slice.Members)
	}
}

func TestImportKeepsExistingCreatedSource(t *testing.T) {
	reg := Registry{}
	reg.RegisterCreated("feature", "web", "feature", "/managed/web")
	reg.Import("feature", "api", "feature", "/loose/api")
	if got := reg.Slices["feature"].Source; got != SourceCreated {
		t.Fatalf("source changed to %q, want created", got)
	}
}

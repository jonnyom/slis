package discovery

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestApplyRegroups(t *testing.T) {
	discovered := []model.Slice{
		{
			Name: "checkout",
			Members: map[string]model.SliceMember{
				"web": {
					Repo:         "web",
					Branch:       "jonny/checkout",
					WorktreePath: "/wt/web",
					TipSHA:       "abc",
				},
			},
		},
		{
			Name: "checkout-api",
			Members: map[string]model.SliceMember{
				"api": {
					Repo:         "api",
					Branch:       "jonny/checkout-api",
					WorktreePath: "/wt/api",
					TipSHA:       "def",
				},
			},
		},
	}

	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	got := Apply(discovered, ov)

	// Expect exactly one slice named "checkout".
	if len(got) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(got))
	}
	if got[0].Name != "checkout" {
		t.Fatalf("expected slice name %q, got %q", "checkout", got[0].Name)
	}

	// Expect both web and api members.
	if len(got[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got[0].Members))
	}

	// Verify web member is intact.
	web, ok := got[0].Members["web"]
	if !ok {
		t.Fatal("expected member 'web' in checkout slice")
	}
	if web.WorktreePath != "/wt/web" || web.TipSHA != "abc" {
		t.Errorf("web member unexpected: %+v", web)
	}

	// Verify api member was moved and kept its original fields.
	api, ok := got[0].Members["api"]
	if !ok {
		t.Fatal("expected member 'api' in checkout slice")
	}
	if api.WorktreePath != "/wt/api" {
		t.Errorf("api WorktreePath: want %q, got %q", "/wt/api", api.WorktreePath)
	}
	if api.TipSHA != "def" {
		t.Errorf("api TipSHA: want %q, got %q", "def", api.TipSHA)
	}
}

func TestSaveLoadOverridesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	if err := SaveOverrides(path, ov); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("LoadOverrides: %v", err)
	}

	if !reflect.DeepEqual(got, ov) {
		t.Errorf("round-trip mismatch:\n  got  %v\n  want %v", got, ov)
	}
}

func TestLoadOverridesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.yaml")

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty Overrides, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty Overrides, got %v", got)
	}
}

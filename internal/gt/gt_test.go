package gt_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/gt"
)

func TestParseStateFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/state.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	state, err := gt.ParseState(data)
	if err != nil {
		t.Fatalf("ParseState: %v", err)
	}

	main, ok := state["main"]
	if !ok {
		t.Fatal("missing branch 'main'")
	}
	if !main.Trunk {
		t.Errorf("main.Trunk = false; want true")
	}

	feat, ok := state["feat"]
	if !ok {
		t.Fatal("missing branch 'feat'")
	}
	if feat.Trunk {
		t.Errorf("feat.Trunk = true; want false")
	}
	if feat.NeedsRestack {
		t.Errorf("feat.NeedsRestack = true; want false")
	}
	if len(feat.Parents) != 1 {
		t.Fatalf("len(feat.Parents) = %d; want 1", len(feat.Parents))
	}
	if feat.Parents[0].Ref != "main" {
		t.Errorf("feat.Parents[0].Ref = %q; want %q", feat.Parents[0].Ref, "main")
	}
	const wantSHA = "a31473161e2901cc829000ab9ba61280ec97b029"
	if feat.Parents[0].SHA != wantSHA {
		t.Errorf("feat.Parents[0].SHA = %q; want %q", feat.Parents[0].SHA, wantSHA)
	}
}

func TestStripBanner(t *testing.T) {
	input := []byte("warn line1\nwarn line2\n{\"main\":{\"trunk\":true}}")
	stripped := gt.StripBanner(input)
	if len(stripped) == 0 || stripped[0] != '{' {
		t.Fatalf("StripBanner result does not start with '{': %q", stripped)
	}

	state, err := gt.ParseState(stripped)
	if err != nil {
		t.Fatalf("ParseState after StripBanner: %v", err)
	}
	if !state["main"].Trunk {
		t.Errorf("main.Trunk = false; want true")
	}
}

func TestOrdered(t *testing.T) {
	data, err := os.ReadFile("testdata/state.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	state, err := gt.ParseState(data)
	if err != nil {
		t.Fatalf("ParseState: %v", err)
	}

	ordered := state.Ordered()
	if len(ordered) < 2 {
		t.Fatalf("Ordered() returned %d items; want at least 2", len(ordered))
	}

	if ordered[0].Name != "main" {
		t.Errorf("ordered[0].Name = %q; want %q", ordered[0].Name, "main")
	}
	if ordered[0].Depth != 0 {
		t.Errorf("ordered[0].Depth = %d; want 0", ordered[0].Depth)
	}
	if !ordered[0].Trunk {
		t.Errorf("ordered[0].Trunk = false; want true")
	}

	if ordered[1].Name != "feat" {
		t.Errorf("ordered[1].Name = %q; want %q", ordered[1].Name, "feat")
	}
	if ordered[1].Depth != 1 {
		t.Errorf("ordered[1].Depth = %d; want 1", ordered[1].Depth)
	}
	if ordered[1].Trunk {
		t.Errorf("ordered[1].Trunk = true; want false")
	}
}

func TestReadStateSkipsWithoutGt(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt is installed; skipping no-gt test")
	}
	// gt is not installed — ReadState must return gracefully (no panic).
	_, err := gt.ReadState(".")
	// We allow either nil or a non-panic error here; the key requirement is no panic.
	_ = err
}

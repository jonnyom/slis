package discovery

import (
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestUnitsSurviveGroupingSavesAndAnnotateSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")

	if err := SaveUnits(path, Units{"wfm-mw-lean": "wfm-mw-latency", "wfm-mw-latency": "wfm-mw-latency"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveOverrides(path, Overrides{"checkout": {"web": "jonny/checkout"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(path, Overrides{}, Folded{"tip": {"web": {"jonny/mid"}}}); err != nil {
		t.Fatal(err)
	}

	units, err := LoadUnits(path)
	if err != nil {
		t.Fatal(err)
	}
	if units["wfm-mw-lean"] != "wfm-mw-latency" {
		t.Fatalf("units lost after grouping saves: %v", units)
	}
	if folded, _ := LoadFolded(path); len(folded["tip"]["web"]) != 1 {
		t.Fatalf("SaveUnits dropped folded section: %v", folded)
	}

	slices := ApplyUnits([]model.Slice{{Name: "wfm-mw-lean"}, {Name: "solo"}}, units)
	if slices[0].Unit != "wfm-mw-latency" || slices[1].Unit != "" {
		t.Fatalf("ApplyUnits = %+v", slices)
	}
}

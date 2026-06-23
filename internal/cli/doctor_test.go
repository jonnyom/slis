package cli

import "testing"

func TestStripPrefixFinding(t *testing.T) {
	if f := stripPrefixFinding(""); f != nil {
		t.Errorf("empty prefix should be OK, got %+v", f)
	}
	if f := stripPrefixFinding("jonny/"); f != nil {
		t.Errorf("trailing-slash prefix should be OK, got %+v", f)
	}
	f := stripPrefixFinding("jonny")
	if f == nil || f.Level != lvlWarn {
		t.Errorf("slashless prefix should warn, got %+v", f)
	}
}

func TestDoubledPrefixFinding(t *testing.T) {
	if f := doubledPrefixFinding("s", "r", "jonny/wfm-1", "jonny"); f != nil {
		t.Errorf("single prefix should be OK, got %+v", f)
	}
	if f := doubledPrefixFinding("s", "r", "wfm-1", ""); f != nil {
		t.Errorf("empty prefix should be OK, got %+v", f)
	}
	f := doubledPrefixFinding("s", "r", "jonnyjonny/wfm-1", "jonny")
	if f == nil || f.Level != lvlFail {
		t.Errorf("doubled prefix should fail, got %+v", f)
	}
}

func TestDeDoubledBranch(t *testing.T) {
	cases := []struct {
		branch, prefix, want string
	}{
		{"jonnyjonny/wfm-1", "jonny", "jonny/wfm-1"},
		{"jonny/jonny/wfm-1", "jonny/", "jonny/wfm-1"},
		{"jonny/wfm-1", "jonny", ""}, // not doubled
		{"wfm-1", "", ""},            // no prefix
	}
	for _, c := range cases {
		if got := deDoubledBranch(c.branch, c.prefix); got != c.want {
			t.Errorf("deDoubledBranch(%q, %q) = %q, want %q", c.branch, c.prefix, got, c.want)
		}
	}
}

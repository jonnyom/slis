package cli

import "testing"

func TestValidateSliceName(t *testing.T) {
	valid := []string{
		"feature",
		"feat/sub",
		"a/b/c",
		"my-feature_1.2",
		"JIRA-123",
	}
	for _, n := range valid {
		if err := validateSliceName(n); err != nil {
			t.Errorf("validateSliceName(%q) = %v, want nil", n, err)
		}
	}

	invalid := []string{
		"",            // empty
		"-rf",         // leading dash → looks like a git flag
		"/abs/path",   // absolute
		"../escape",   // traversal
		"a/../b",      // traversal mid-path
		"a/./b",       // dot segment
		"a//b",        // empty segment
		"trailing/",   // trailing slash → empty segment
		`back\slash`,  // backslash
		"ctl\x1bchar", // control character (ESC)
		"nul\x00",     // NUL
	}
	for _, n := range invalid {
		if err := validateSliceName(n); err == nil {
			t.Errorf("validateSliceName(%q) = nil, want error", n)
		}
	}
}

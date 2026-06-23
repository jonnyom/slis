package notify

import (
	"strings"
	"testing"
)

// TestEscapeAppleScript verifies that strings cannot break out of the
// AppleScript double-quoted literal they are embedded in.
func TestEscapeAppleScript(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "alpha needs input", "alpha needs input"},
		{"double quote escaped", `say "hi"`, `say \"hi\"`},
		{"backslash escaped", `a\b`, `a\\b`},
		{"newline dropped", "line1\nline2", "line1line2"},
		{"ESC dropped", "a\x1bb", "ab"},
		{"injection attempt", `x" & (do shell script "calc") & "y`, `x\" & (do shell script \"calc\") & \"y`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := escapeAppleScript(c.in)
			if got != c.want {
				t.Errorf("escapeAppleScript(%q) = %q, want %q", c.in, got, c.want)
			}
			// No raw (unescaped) double-quote may survive — that is the break-out byte.
			if strings.Contains(strings.ReplaceAll(got, `\"`, ""), `"`) {
				t.Errorf("escapeAppleScript(%q) left an unescaped quote: %q", c.in, got)
			}
		})
	}
}

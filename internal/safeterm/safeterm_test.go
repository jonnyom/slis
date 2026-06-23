package safeterm

import "testing"

func TestStrip(t *testing.T) {
	// U+0085 (NEL) and U+009B (CSI) are C1 controls; built from rune values so
	// they are valid UTF-8 in the string and must be stripped.
	c1 := string([]rune{'a', 0x85, 0x9b, 'b'})

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text untouched", "hello world", "hello world"},
		{"unicode untouched", "café — naïve ✅", "café — naïve ✅"},
		{"keeps newline/tab/cr", "a\nb\tc\rd", "a\nb\tc\rd"},
		{"strips ESC (ANSI lead byte)", "a\x1b[31mred\x1b[0m", "a[31mred[0m"},
		{"strips OSC 52 clipboard hijack", "x\x1b]52;c;ZXZpbA==\x07y", "x]52;c;ZXZpbA==y"},
		{"strips other C0 controls", "a\x00\x01\x07b", "ab"},
		{"strips C1 controls (valid UTF-8)", c1, "ab"},
		{"strips DEL", "a\x7fb", "ab"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Strip(c.in); got != c.want {
				t.Errorf("Strip(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

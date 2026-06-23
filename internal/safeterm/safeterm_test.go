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

func TestStripNonSGR(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"keeps SGR colour", "a\x1b[31mred\x1b[0mb", "a\x1b[31mred\x1b[0mb"},
		{"strips cursor move (CSI non-m)", "a\x1b[2Jb\x1b[10;5Hc", "abc"},
		{"strips OSC 52 clipboard (BEL)", "x\x1b]52;c;ZXZ==\x07y", "xy"},
		{"strips OSC (ST terminated)", "x\x1b]0;title\x1b\\y", "xy"},
		{"strips lone ESC + charset seq", "a\x1b(Bb", "ab"},
		{"keeps newline/tab, drops other C0", "a\nb\tc\x00d", "a\nb\tcd"},
		{"plain text untouched", "hello", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripNonSGR(c.in); got != c.want {
				t.Errorf("StripNonSGR(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Package safeterm sanitises strings that originate outside slis (GitHub PR
// titles, commit messages, diff contents, captured pane text, …) before they
// are written to the terminal.
//
// Such strings can carry ANSI/OSC escape sequences. Rendered raw they let an
// attacker move the cursor, clear or spoof the screen, or hijack the clipboard
// (OSC 52). This matters for slis specifically because it displays data from
// repos you cloned and PRs opened by other people. Strip removes C0/C1 control
// bytes (keeping the few that are part of normal layout) so the worst a hostile
// string can do is show odd glyphs.
package safeterm

import "strings"

// Strip removes ASCII C0 control characters (0x00–0x1F) and C1 control
// characters (0x80–0x9F) from s, preserving newline (\n), tab (\t) and
// carriage-return (\r) which are legitimate layout characters. ESC (0x1B) — the
// lead byte of every ANSI/OSC sequence — is therefore removed, neutralising
// escape injection while leaving ordinary printable text untouched.
func Strip(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t', '\r':
			return r
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

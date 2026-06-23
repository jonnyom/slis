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

// StripNonSGR removes terminal escape sequences EXCEPT SGR colour sequences
// (ESC[…m), and drops C0 control bytes other than \n, \t, \r. Use it for content
// from the user's OWN terminal session (e.g. a captured tmux pane) where colour
// should be preserved but cursor moves, OSC clipboard/title writes, and other
// escapes should not be honoured.
func StripNonSGR(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b: // ESC — start of an escape sequence
			seq, adv, keep := scanEscape(s[i:])
			if keep {
				b.WriteString(seq)
			}
			if adv < 1 {
				adv = 1
			}
			i += adv
		case c == '\n' || c == '\t' || c == '\r':
			b.WriteByte(c)
			i++
		case c < 0x20 || c == 0x7f:
			i++ // drop other C0 control + DEL
		default:
			b.WriteByte(c) // printable / UTF-8 byte
			i++
		}
	}
	return b.String()
}

// scanEscape inspects an escape sequence beginning at s[0]==ESC. It returns the
// sequence text, how many bytes it spans, and whether to keep it (only SGR
// colour sequences, ESC[…m, are kept).
func scanEscape(s string) (seq string, advance int, keep bool) {
	if len(s) < 2 {
		return "", 1, false // lone ESC
	}
	switch s[1] {
	case '[': // CSI: ESC [ params… final(0x40–0x7e)
		for j := 2; j < len(s); j++ {
			if b := s[j]; b >= 0x40 && b <= 0x7e {
				return s[:j+1], j + 1, b == 'm' // keep only SGR
			}
		}
		return "", len(s), false // unterminated
	case ']': // OSC: ESC ] … (BEL or ST) — always stripped
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 { // BEL
				return "", j + 1, false
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' { // ST
				return "", j + 2, false
			}
		}
		return "", len(s), false
	default:
		// nF-style escapes: ESC, zero or more intermediates (0x20–0x2f), one
		// final byte (e.g. ESC ( B to select a charset, or ESC 7 / ESC M).
		j := 1
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j < len(s) {
			j++ // consume the final byte
		}
		return "", j, false
	}
}

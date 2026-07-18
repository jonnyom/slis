// tmux capture lines keep SGR colour escapes (the sidecar strips only
// cursor/OSC control sequences). OpenTUI's <text> renders content literally and
// does not interpret embedded ANSI, so we strip the remaining SGR codes to
// plain text at the render boundary. (Wave 2 can parse SGR into styled spans.)

// eslint-disable-next-line no-control-regex
const SGR_RE = /\x1b\[[0-9;]*m/g;

export function stripSgr(line: string): string {
  return line.replace(SGR_RE, "");
}

export function stripSgrLines(lines: string[]): string[] {
  return lines.map(stripSgr);
}

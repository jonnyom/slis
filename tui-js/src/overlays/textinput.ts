// Minimal controlled-text-input reducer. The app owns a single global keyboard
// subscription, so text overlays (create slice, group name, search) edit their
// string through this pure reducer rather than a focus-stealing input widget.

export interface EditKey {
  name: string;
  sequence?: string;
  ctrl?: boolean;
  meta?: boolean;
}

// Returns the next text value given a keypress. Printable single characters are
// appended (via the raw sequence, so shift/space/punctuation land correctly);
// backspace trims one; ctrl-u clears the line. Everything else is a no-op — the
// caller handles enter/escape as submit/cancel before delegating here.
export function editText(text: string, key: EditKey): string {
  if (key.ctrl || key.meta) {
    if (key.name === "u") return "";
    return text;
  }
  if (key.name === "backspace") return text.slice(0, -1);
  const seq = key.sequence ?? "";
  if (seq.length === 1) {
    const code = seq.charCodeAt(0);
    if (code >= 0x20 && code !== 0x7f) return text + seq;
  }
  return text;
}

// Soft-wrap a controlled input and retain the tail that fits in its capped
// viewport. Since the caret is append-only, this is the terminal equivalent of
// automatically scrolling the textarea as the comment grows.
export function visibleTextLines(text: string, width: number, maxRows: number): string[] {
  const columns = Math.max(1, width);
  const rows = Math.max(1, maxRows);
  if (text.length === 0) return [""];

  const lines: string[] = [];
  let rest = text;
  while (rest.length > columns) {
    const candidate = rest.slice(0, columns + 1);
    const space = candidate.lastIndexOf(" ");
    const cut = space > 0 ? space : columns;
    lines.push(rest.slice(0, cut));
    rest = rest.slice(cut);
    if (rest.startsWith(" ")) rest = rest.slice(1);
  }
  lines.push(rest);
  return lines.slice(-rows);
}

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

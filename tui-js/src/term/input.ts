const KITTY_SHIFT_ENTER = "\x1b[13;2u";
const BRACKETED_PASTE_START = new TextEncoder().encode("\x1b[200~");
const BRACKETED_PASTE_END = new TextEncoder().encode("\x1b[201~");

export function embeddedTerminalInputSequence(sequence: string): string {
  return sequence === KITTY_SHIFT_ENTER ? "\n" : sequence;
}

export function embeddedTerminalPasteSequence(bytes: Uint8Array): Uint8Array {
  const sequence = new Uint8Array(
    BRACKETED_PASTE_START.length + bytes.length + BRACKETED_PASTE_END.length,
  );
  sequence.set(BRACKETED_PASTE_START);
  sequence.set(bytes, BRACKETED_PASTE_START.length);
  sequence.set(BRACKETED_PASTE_END, BRACKETED_PASTE_START.length + bytes.length);
  return sequence;
}

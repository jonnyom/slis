const QUERIES = [
  { query: "\x1b]10;?\x1b\\", response: "\x1b]10;rgb:ffff/ffff/ffff\x1b\\" },
  { query: "\x1b]11;?\x1b\\", response: "\x1b]11;rgb:0000/0000/0000\x1b\\" },
  { query: "\x1b[6n", response: "\x1b[1;1R" },
  { query: "\x1b[c", response: "\x1b[?1;2c" },
  { query: "\x1b[>c", response: "\x1b[>0;10;1c" },
] as const;

const MAX_QUERY_LENGTH = Math.max(...QUERIES.map(({ query }) => query.length));

export class TerminalQueryResponder {
  private pending = "";
  private readonly decoder = new TextDecoder();

  observe(bytes: Uint8Array): string[] {
    this.pending += this.decoder.decode(bytes, { stream: true });
    const responses: string[] = [];
    for (const { query, response } of QUERIES) {
      let index = this.pending.indexOf(query);
      while (index >= 0) {
        responses.push(response);
        this.pending = this.pending.slice(0, index) + this.pending.slice(index + query.length);
        index = this.pending.indexOf(query);
      }
    }
    if (this.pending.length >= MAX_QUERY_LENGTH) {
      this.pending = this.pending.slice(-(MAX_QUERY_LENGTH - 1));
    }
    return responses;
  }
}

export class TerminalModeTracker {
  private pending = "";
  private readonly decoder = new TextDecoder();
  private readonly enabledMouseModes = new Set<number>();
  bracketedPaste = false;
  applicationHandlesMouse = false;

  observe(bytes: Uint8Array): void {
    this.pending += this.decoder.decode(bytes, { stream: true });
    const enabled = this.pending.lastIndexOf("\x1b[?2004h");
    const disabled = this.pending.lastIndexOf("\x1b[?2004l");
    if (enabled >= 0 || disabled >= 0) this.bracketedPaste = enabled > disabled;
    for (const match of this.pending.matchAll(/\x1b\[\?(1000|1002|1003)(h|l)/g)) {
      const mode = Number(match[1]);
      if (match[2] === "h") this.enabledMouseModes.add(mode);
      else this.enabledMouseModes.delete(mode);
    }
    this.applicationHandlesMouse = this.enabledMouseModes.size > 0;
    if (this.pending.length > 15) this.pending = this.pending.slice(-15);
  }
}

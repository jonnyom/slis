import { stripAnsiSequences } from "@opentui/core";

const VISIBLE_URL = /https?:\/\/[^\s<>"']+/g;
const TRAILING_PUNCTUATION = /[.,;:!?)}\]]+$/;
const OSC_8_LINK = /\x1b\]8;[^;\x07\x1b]*;([^\x07\x1b]*)(?:\x07|\x1b\\)([\s\S]*?)\x1b\]8;;(?:\x07|\x1b\\)/g;
const OSC_8_START = "\x1b]8;";
const MAX_PENDING_LINK_BYTES = 64 * 1024;
const MAX_TRACKED_LINKS = 256;
const CLICKABLE_PROTOCOLS = new Set(["file:", "http:", "https:"]);

export interface TerminalLinkAlias {
  text: string;
  url: string;
}

function textContainsPosition(line: string, text: string, column: number): boolean {
  let startIndex = line.indexOf(text);
  while (startIndex >= 0) {
    const startColumn = Bun.stringWidth(line.slice(0, startIndex));
    const endColumn = startColumn + Bun.stringWidth(text);
    if (column >= startColumn && column < endColumn) return true;
    startIndex = line.indexOf(text, startIndex + text.length);
  }
  return false;
}

function hasClickableProtocol(url: string): boolean {
  return URL.canParse(url) && CLICKABLE_PROTOCOLS.has(new URL(url).protocol);
}

export function visibleUrlAtPosition(
  terminalText: string,
  row: number,
  column: number,
): string | null {
  const line = terminalText.split("\n")[row];
  if (line === undefined) return null;

  for (const match of line.matchAll(VISIBLE_URL)) {
    const url = match[0].replace(TRAILING_PUNCTUATION, "");
    const startColumn = Bun.stringWidth(line.slice(0, match.index));
    const endColumn = startColumn + Bun.stringWidth(url);
    if (column >= startColumn && column < endColumn) return url;
  }

  return null;
}

export class Osc8LinkTracker {
  private decoder = new TextDecoder();
  private readonly aliasesByText = new Map<string, string | null>();
  private pending = "";

  observe(data: string | Buffer | Uint8Array): void {
    this.pending += typeof data === "string"
      ? data
      : this.decoder.decode(data, { stream: true });

    OSC_8_LINK.lastIndex = 0;
    let match: RegExpExecArray | null;
    let consumed = 0;
    while ((match = OSC_8_LINK.exec(this.pending)) !== null) {
      const url = match[1] ?? "";
      const text = stripAnsiSequences(match[2] ?? "").replaceAll("\r", "");
      if (
        hasClickableProtocol(url) &&
        text &&
        !text.includes("\n")
      ) {
        this.remember(text, url);
      }
      consumed = OSC_8_LINK.lastIndex;
    }

    const unconsumed = this.pending.slice(consumed);
    const partialLinkStart = unconsumed.indexOf(OSC_8_START);
    let partialLink = partialLinkStart >= 0 ? unconsumed.slice(partialLinkStart) : "";
    if (partialLinkStart < 0) {
      for (let length = OSC_8_START.length - 1; length > 0; length--) {
        if (unconsumed.endsWith(OSC_8_START.slice(0, length))) {
          partialLink = unconsumed.slice(-length);
          break;
        }
      }
    }
    this.pending = partialLink.length <= MAX_PENDING_LINK_BYTES ? partialLink : "";
  }

  urlAtPosition(terminalText: string, row: number, column: number): string | null {
    const line = terminalText.split("\n")[row];
    if (line === undefined) return null;
    const aliases = [...this.aliasesByText].reverse();
    for (const [text, url] of aliases) {
      if (url && textContainsPosition(line, text, column)) return url;
    }
    return null;
  }

  get aliases(): TerminalLinkAlias[] {
    return [...this.aliasesByText].flatMap(([text, url]) =>
      url ? [{ text, url }] : [],
    );
  }

  reset(): void {
    this.decoder = new TextDecoder();
    this.pending = "";
    this.aliasesByText.clear();
  }

  private remember(text: string, url: string): void {
    const existingUrl = this.aliasesByText.get(text);
    this.aliasesByText.delete(text);
    this.aliasesByText.set(text, existingUrl === undefined || existingUrl === url ? url : null);
    const oldestText = this.aliasesByText.keys().next().value;
    if (this.aliasesByText.size > MAX_TRACKED_LINKS && oldestText) {
      this.aliasesByText.delete(oldestText);
    }
  }
}

export function activateVisibleUrlAtPosition(
  terminalText: string,
  row: number,
  column: number,
  onOpen: (url: string) => void,
): boolean {
  const url = visibleUrlAtPosition(terminalText, row, column);
  if (!url) return false;
  onOpen(url);
  return true;
}

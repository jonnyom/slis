const VISIBLE_URL = /https?:\/\/[^\s<>"']+/g;
const TRAILING_PUNCTUATION = /[.,;:!?)}\]]+$/;

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

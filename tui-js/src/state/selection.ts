// Pure selection + search helpers for the browser's batch-op / filter modes.
// Kept side-effect-free so they can be unit-tested without a terminal.

export function toggleSelected(
  selected: ReadonlySet<string>,
  name: string,
): Set<string> {
  const next = new Set(selected);
  if (next.has(name)) next.delete(name);
  else next.add(name);
  return next;
}

// Mirrors the Bubble Tea `A` key: if every currently-visible slice is already
// selected, clear that visible set; otherwise add all of them. Scoped to the
// filtered/visible list, never the whole slice set.
export function toggleAllVisible(
  selected: ReadonlySet<string>,
  visible: readonly string[],
): Set<string> {
  const next = new Set(selected);
  const allSelected = visible.length > 0 && visible.every((n) => next.has(n));
  if (allSelected) for (const n of visible) next.delete(n);
  else for (const n of visible) next.add(n);
  return next;
}

// Substring, case-insensitive match on the slice name only — the same rule the
// Go browser uses for its `/` incremental filter.
export function matchesSearch(name: string, query: string): boolean {
  if (query === "") return true;
  return name.toLowerCase().includes(query.toLowerCase());
}

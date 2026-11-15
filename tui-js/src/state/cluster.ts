// Browser slice-list ordering helpers, ported from internal/tui/slicelist.go:
//   - clusterByStack: group stack-sibling slices adjacently under a header,
//     matching clusterVisByStack (trunk-first by stack_order, groups keep their
//     first-appearance position).
//   - nextAttentionIndex: n/N inbox navigation over the attention slices, matching
//     attentionOrder + the n/N key handlers.
// Pure and side-effect-free so they can be unit-tested without a terminal.

import { attentionRank, type SliceView } from "./derive";

// stackRootOf extracts the root branch name from a slice's stack_id, which the
// sidecar encodes as "<repo>\x00<root>" (a NUL byte separator). Returns "" when
// there is no annotation. Mirrors slicelist.go's stackRootOf.
export function stackRootOf(stackId: string | undefined): string {
  if (!stackId) return "";
  const i = stackId.indexOf("\x00");
  return i >= 0 ? stackId.slice(i + 1) : "";
}

export interface StackLeader {
  root: string;
  count: number;
}

export interface UnitLeader {
  unit: string;
  count: number;
}

export interface Clustered {
  ordered: SliceView[];
  // slice name → header shown above it (only for the leader of a group > 1).
  leaders: Map<string, StackLeader>;
}

export interface UnitClustered extends Clustered {
  // slice name → unit header shown above it (the first slice of each unit).
  unitLeaders: Map<string, UnitLeader>;
}

// clusterByUnit lists loose slices first (clustered by stack as before), then
// each unit in first-appearance order, its members kept adjacent under a unit
// header with stack clustering inside.
export function clusterByUnit(views: SliceView[]): UnitClustered {
  const buckets = new Map<string, SliceView[]>();
  views.forEach((v, i) => {
    const key = v.slice.unit
      ? `unit:${v.slice.unit}`
      : v.slice.stack_id
        ? `stack:${v.slice.stack_id}`
        : `\x00solo-${i}`;
    if (!buckets.has(key)) buckets.set(key, []);
    buckets.get(key)!.push(v);
  });

  const ordered: SliceView[] = [];
  const leaders = new Map<string, StackLeader>();
  const unitLeaders = new Map<string, UnitLeader>();
  const all = [...buckets.values()];
  const loose = all.filter((bucket) => !bucket[0]!.slice.unit);
  const units = all.filter((bucket) => !!bucket[0]!.slice.unit);
  for (const bucket of [...loose, ...units]) {
    const c = clusterByStack(bucket);
    const unit = bucket[0]!.slice.unit;
    if (unit) unitLeaders.set(c.ordered[0]!.slice.name, { unit, count: c.ordered.length });
    ordered.push(...c.ordered);
    c.leaders.forEach((leader, name) => leaders.set(name, leader));
  }
  return { ordered, leaders, unitLeaders };
}

// clusterByStack reorders views so slices sharing a stack_id sit adjacently,
// trunk-first by stack_order, while preserving each group's first-appearance
// position. Slices with no stack_id keep their place. The leaders map carries a
// header (root branch + count) for the first slice of every group larger than one.
export function clusterByStack(views: SliceView[]): Clustered {
  const groups = new Map<string, SliceView[]>();
  const order: string[] = [];
  views.forEach((v, i) => {
    let key = v.slice.stack_id ?? "";
    if (key === "") key = `\x00solo-${i}`; // unique: stays in place, no header
    if (!groups.has(key)) {
      groups.set(key, []);
      order.push(key);
    }
    groups.get(key)!.push(v);
  });

  const ordered: SliceView[] = [];
  const leaders = new Map<string, StackLeader>();
  for (const key of order) {
    const g = groups.get(key)!;
    if (g.length > 1) {
      g.sort(
        (a, b) => (a.slice.stack_order ?? 0) - (b.slice.stack_order ?? 0),
      );
      const lead = g[0]!;
      if (lead.slice.stack_id) {
        leaders.set(lead.slice.name, {
          root: stackRootOf(lead.slice.stack_id),
          count: g.length,
        });
      }
    }
    ordered.push(...g);
  }
  return { ordered, leaders };
}

export function isGatherableStackSlice(views: SliceView[], sliceName: string): boolean {
  const stackId = views.find((view) => view.slice.name === sliceName)?.slice.stack_id;
  if (!stackId) return false;
  return views.filter((view) => view.slice.stack_id === stackId).length > 1;
}

// attentionIndices returns the positions in views of slices that need attention
// (attentionRank < 99), preserving the list's own order.
export function attentionIndices(views: SliceView[]): number[] {
  const idxs: number[] = [];
  views.forEach((v, i) => {
    if (attentionRank(v) < 99) idxs.push(i);
  });
  return idxs;
}

// nextAttentionIndex jumps to the next (dir=1) / previous (dir=-1) attention
// slice relative to `current`, wrapping around. Returns null when nothing needs
// attention. Mirrors the Go n/N handlers: when the cursor is not already on an
// attention slice, n lands on the first and N on the last.
export function nextAttentionIndex(
  views: SliceView[],
  current: number,
  dir: 1 | -1,
): number | null {
  const idxs = attentionIndices(views);
  if (idxs.length === 0) return null;
  const p = idxs.indexOf(current);
  if (p < 0) return dir === 1 ? idxs[0]! : idxs[idxs.length - 1]!;
  const np = (p + dir + idxs.length) % idxs.length;
  return idxs[np]!;
}

// A browser row. Slices inside a unit carry `unit` so the list can draw them
// under the unit header's tree gutter; `last` marks the closing member, which is
// what tells the eye where the unit ends and the next slice begins.
export type BrowserRow =
  | { kind: "slice"; view: SliceView; unit?: { name: string; last: boolean } }
  | { kind: "unit"; unit: string; members: SliceView[]; collapsed: boolean }
  | { kind: "missing"; name: string };

export interface UnitRowOptions {
  unitLeaders: Map<string, UnitLeader>;
  collapsed: ReadonlySet<string>;
  // Search shows every match, so a hit inside a collapsed unit stays visible.
  expandAll?: boolean;
}

export function missingSliceNames(
  missing: { slice: string }[] | undefined,
): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const m of missing ?? []) {
    if (!seen.has(m.slice)) {
      seen.add(m.slice);
      out.push(m.slice);
    }
  }
  return out;
}

// buildRows lays the clustered slices out as rows. Each unit gets its own header
// row (selectable, so it can be collapsed) followed by its members, which
// clusterByUnit has already placed contiguously after the unit leader.
export function buildRows(
  ordered: SliceView[],
  missing: string[],
  units?: UnitRowOptions,
): BrowserRow[] {
  const rows: BrowserRow[] = [];
  for (let i = 0; i < ordered.length; i++) {
    const view = ordered[i]!;
    const leader = units?.unitLeaders.get(view.slice.name);
    if (!leader) {
      rows.push({ kind: "slice", view });
      continue;
    }
    const members = ordered.slice(i, i + leader.count);
    const collapsed = !units!.expandAll && units!.collapsed.has(leader.unit);
    rows.push({ kind: "unit", unit: leader.unit, members, collapsed });
    if (!collapsed) {
      members.forEach((member, j) =>
        rows.push({
          kind: "slice",
          view: member,
          unit: { name: leader.unit, last: j === members.length - 1 },
        }),
      );
    }
    i += members.length - 1;
  }
  for (const name of missing) rows.push({ kind: "missing", name });
  return rows;
}

function isSelectable(row: BrowserRow | undefined): boolean {
  return row?.kind === "slice" || row?.kind === "unit";
}

export function selectableIndices(rows: BrowserRow[]): number[] {
  const out: number[] = [];
  rows.forEach((r, i) => {
    if (isSelectable(r)) out.push(i);
  });
  return out;
}

// unitAttention is the most urgent attention rank among a unit's members, so a
// collapsed unit can still say "something in here needs you".
export function unitAttention(members: SliceView[]): number {
  return Math.min(99, ...members.map(attentionRank));
}

// unitOfRow names the unit a row belongs to: the header itself or a member.
export function unitOfRow(row: BrowserRow | undefined): string | null {
  if (row?.kind === "unit") return row.unit;
  if (row?.kind === "slice") return row.unit?.name ?? null;
  return null;
}

// unitHeaderIndex finds the header row of a unit, used to park focus on it when
// the unit is collapsed from one of its members.
export function unitHeaderIndex(rows: BrowserRow[], unit: string): number {
  return rows.findIndex((row) => row.kind === "unit" && row.unit === unit);
}

// collapsedUnitOf reports the unit that hides a slice when collapsed, so a focus
// request for that slice can expand it first.
export function collapsedUnitOf(rows: BrowserRow[], sliceName: string): string | null {
  for (const row of rows) {
    if (row.kind === "unit" && row.collapsed && row.members.some((m) => m.slice.name === sliceName))
      return row.unit;
  }
  return null;
}

export function firstSelectable(rows: BrowserRow[]): number {
  return selectableIndices(rows)[0] ?? 0;
}

export function focusIndexForSlice(
  rows: BrowserRow[],
  sliceName: string | null | undefined,
): number {
  if (sliceName) {
    const index = rows.findIndex(
      (row) => row.kind === "slice" && row.view.slice.name === sliceName,
    );
    if (index >= 0) return index;
  }
  return firstSelectable(rows);
}

export function lastSelectable(rows: BrowserRow[]): number {
  const s = selectableIndices(rows);
  return s.length ? s[s.length - 1]! : 0;
}

export function stepSelectable(
  rows: BrowserRow[],
  current: number,
  dir: 1 | -1,
): number {
  const sel = selectableIndices(rows);
  if (sel.length === 0) return current;
  const pos = sel.indexOf(current);
  if (pos < 0) {
    if (dir === 1) return sel.find((i) => i > current) ?? sel[sel.length - 1]!;
    const below = sel.filter((i) => i < current);
    return below.length ? below[below.length - 1]! : sel[0]!;
  }
  const np = Math.max(0, Math.min(sel.length - 1, pos + dir));
  return sel[np]!;
}

export function clampFocus(rows: BrowserRow[], current: number): number {
  const sel = selectableIndices(rows);
  if (sel.length === 0) return 0;
  if (isSelectable(rows[current])) return current;
  const below = sel.filter((i) => i <= current);
  return below.length ? below[below.length - 1]! : sel[0]!;
}

export function attentionRowIndices(rows: BrowserRow[]): number[] {
  const out: number[] = [];
  rows.forEach((r, i) => {
    if (r.kind === "slice" && attentionRank(r.view) < 99) out.push(i);
    else if (r.kind === "unit" && r.collapsed && unitAttention(r.members) < 99) out.push(i);
  });
  return out;
}

export function nextAttentionRow(
  rows: BrowserRow[],
  current: number,
  dir: 1 | -1,
): number | null {
  const idxs = attentionRowIndices(rows);
  if (idxs.length === 0) return null;
  const p = idxs.indexOf(current);
  if (p < 0) return dir === 1 ? idxs[0]! : idxs[idxs.length - 1]!;
  return idxs[(p + dir + idxs.length) % idxs.length]!;
}

// n/N navigation is search-modal (M3). With an active search the list is already
// narrowed to matches, so n/N step through those matches — the vim/less
// search-repeat reflex. With no search they hop between attention slices (vim's
// `n` is meaningless without a search, so there's no collision). Returns the
// next focus row, or null to leave focus where it is.
export function searchAwareNav(
  searchActive: boolean,
  rows: BrowserRow[],
  current: number,
  dir: 1 | -1,
): number | null {
  return searchActive
    ? stepSelectable(rows, current, dir)
    : nextAttentionRow(rows, current, dir);
}

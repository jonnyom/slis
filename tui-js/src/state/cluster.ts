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

export interface Clustered {
  ordered: SliceView[];
  // slice name → header shown above it (only for the leader of a group > 1).
  leaders: Map<string, StackLeader>;
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

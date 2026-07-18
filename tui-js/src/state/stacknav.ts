// Pure selection model for the cockpit Stack panel. The panel lists, per member
// repo, that repo's Graphite lineage (trunk → … → member branch → upstack). F3
// makes EVERY branch node selectable: j/k walk a flat list of branch rows across
// all repos, and the selected row drives the right pane (the member branch keeps
// the working/parent/trunk scopes; any other branch shows its diff-vs-parent).
//
// Kept React-free so the flattening + clamping is unit-testable.

import type { SliceView } from "./derive";

export interface StackRow {
  repo: string;
  branch: string;
  trunk: boolean;
  needsRestack: boolean;
  // The slice's own branch in this repo (the member). Member rows keep the
  // scope-cycling diff; non-member rows show branch-vs-parent.
  isMember: boolean;
  depth: number;
  // True for the FIRST row of each repo group, so the view draws a repo header.
  firstOfRepo: boolean;
}

// buildStackRows flattens a slice's per-repo lineages into one selectable list,
// member-by-member in slice repo order. A repo with no stack data (gt absent, or
// an untracked branch) contributes a single row for its member branch, so every
// repo is always represented and selectable.
export function buildStackRows(view: SliceView): StackRow[] {
  const rows: StackRow[] = [];
  for (const m of view.slice.members) {
    const stack = view.show?.members.find((s) => s.repo === m.repo)?.stack;
    if (stack && stack.length > 0) {
      stack.forEach((node, i) => {
        rows.push({
          repo: m.repo,
          branch: node.name,
          trunk: node.trunk,
          needsRestack: node.needs_restack,
          isMember: node.name === m.branch,
          depth: node.depth,
          firstOfRepo: i === 0,
        });
      });
    } else {
      rows.push({
        repo: m.repo,
        branch: m.branch,
        trunk: false,
        needsRestack: false,
        isMember: true,
        depth: 0,
        firstOfRepo: true,
      });
    }
  }
  return rows;
}

// clampSel keeps a selection index within [0, len-1] (0 when the list is empty),
// so a shrinking stack (slice/data change) never leaves the cursor out of range.
export function clampSel(sel: number, len: number): number {
  if (len <= 0) return 0;
  return Math.max(0, Math.min(sel, len - 1));
}

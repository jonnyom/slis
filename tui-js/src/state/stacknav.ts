// Pure selection model for the cockpit Stack panel. The panel lists, per member
// repo, that repo's downstack Graphite lineage (trunk → … → member branch). F3
// makes each contextual branch node selectable: j/k walk a flat list of rows
// across all repos, and the selected row drives the right pane (the member branch
// keeps working/parent/trunk scopes; an ancestor shows its diff-vs-parent).
//
// Kept React-free so the flattening + clamping is unit-testable.

import type { SliceView } from "./derive";
import type { DiffRepo } from "../rpc/types";

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
  added?: number;
  deleted?: number;
}

export const MAX_VISIBLE_BRANCHES_PER_REPO = 4;

export interface VisibleStackGroup {
  repo: string;
  rows: Array<{ row: StackRow; index: number }>;
  hiddenBefore: number;
  hiddenAfter: number;
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
          added: node.added,
          deleted: node.deleted,
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

export function withScopedMemberStats(
  rows: StackRow[],
  repos: DiffRepo[] | undefined,
): StackRow[] {
  const reposByName = new Map((repos ?? []).map((repo) => [repo.repo, repo]));
  return rows.map((row) => {
    if (!row.isMember) return row;
    const stat = reposByName.get(row.repo)?.stat;
    return {
      ...row,
      added: stat?.added,
      deleted: stat?.deleted,
    };
  });
}

// compactStackGroups caps each repo without breaking keyboard navigation. The
// trunk/first row stays visible as the repo's inline anchor; when selection
// moves deep into a large stack, a small window follows it and explicit
// overflow counts account for every hidden branch.
export function compactStackGroups(
  rows: StackRow[],
  selected: number,
  cap = MAX_VISIBLE_BRANCHES_PER_REPO,
): VisibleStackGroup[] {
  const groups: Array<Array<{ row: StackRow; index: number }>> = [];
  for (let index = 0; index < rows.length; index++) {
    const item = { row: rows[index]!, index };
    const last = groups[groups.length - 1];
    if (!last || last[0]!.row.repo !== item.row.repo) groups.push([item]);
    else last.push(item);
  }

  return groups.map((items) => {
    const limit = Math.max(1, cap);
    if (items.length <= limit) {
      return { repo: items[0]!.row.repo, rows: items, hiddenBefore: 0, hiddenAfter: 0 };
    }

    const localSelected = items.findIndex((item) => item.index === selected);
    const tailSlots = Math.max(0, limit - 1);
    let start = 1;
    if (tailSlots > 0 && localSelected >= limit) {
      start = Math.max(
        1,
        Math.min(localSelected - Math.floor((tailSlots - 1) / 2), items.length - tailSlots),
      );
    }
    const tail = items.slice(start, start + tailSlots);
    return {
      repo: items[0]!.row.repo,
      rows: [items[0]!, ...tail],
      hiddenBefore: start - 1,
      hiddenAfter: Math.max(0, items.length - (start + tail.length)),
    };
  });
}

export function fitStackGroups(
  rows: StackRow[],
  selected: number,
  availableRows: number,
): VisibleStackGroup[] {
  const largestGroup = rows.reduce((counts, row) => {
    counts.set(row.repo, (counts.get(row.repo) ?? 0) + 1);
    return counts;
  }, new Map<string, number>());
  const maximumCap = Math.max(1, ...largestGroup.values());
  const rowBudget = Math.max(1, availableRows);

  for (let cap = maximumCap; cap >= 1; cap--) {
    const groups = compactStackGroups(rows, selected, cap);
    const renderedRows = groups.reduce(
      (total, group) =>
        total +
        group.rows.length +
        Number(group.hiddenBefore > 0) +
        Number(group.hiddenAfter > 0),
      0,
    );
    if (renderedRows <= rowBudget) return groups;
  }

  return compactStackGroups(rows, selected, 1);
}

// clampSel keeps a selection index within [0, len-1] (0 when the list is empty),
// so a shrinking stack (slice/data change) never leaves the cursor out of range.
export function clampSel(sel: number, len: number): number {
  if (len <= 0) return 0;
  return Math.max(0, Math.min(sel, len - 1));
}

// Trunk rows are useful lineage context but have no parent diff of their own.
// Navigation therefore steps over them and lands only on actionable branches.
export function stepStackBranch(rows: StackRow[], current: number, delta: number): number {
  if (rows.length === 0 || delta === 0) return clampSel(current, rows.length);
  const direction = delta > 0 ? 1 : -1;
  let i = clampSel(current, rows.length);
  for (let n = 0; n < rows.length; n++) {
    i = clampSel(i + direction, rows.length);
    if (!rows[i]?.trunk) return i;
    if (i === 0 || i === rows.length - 1) break;
  }
  return clampSel(current, rows.length);
}

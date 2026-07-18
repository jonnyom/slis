// Pure model for the cockpit's lazy file-tree browser (F3). A branch's tree is
// fetched one directory level per call (the `tree` RPC); this module holds the
// fetched levels + the expanded set and derives the visible flat list the view
// paints. React-free so expand/collapse and flattening are unit-testable.

import type { TreeEntry, TreeEntryType } from "../rpc/types";
import type { FileDiff, FileStatus } from "../diff/parse";

// childrenByPath maps a directory path (repo-relative; "" = root) to the entries
// directly under it, as returned by the `tree` RPC. A path absent from the map
// has not been fetched yet.
export type ChildrenByPath = Record<string, TreeEntry[]>;

export interface FileRow {
  name: string;
  path: string; // full repo-relative path
  type: TreeEntryType;
  size: number;
  depth: number;
  expanded: boolean; // dir only: whether its children are shown
}

// ChangeIndex decorates a branch-revision tree with its diff against the stack
// parent. Exact file paths carry their Git status; ancestor directories are
// tracked separately so collapsed folders still reveal that they contain work.
export interface ChangeIndex {
  files: ReadonlyMap<string, FileStatus>;
  directories: ReadonlySet<string>;
}

export function indexChanges(
  changes: ReadonlyArray<Pick<FileDiff, "path" | "status">>,
): ChangeIndex {
  const files = new Map<string, FileStatus>();
  const directories = new Set<string>();
  for (const change of changes) {
    files.set(change.path, change.status);
    let parent = parentPath(change.path);
    while (parent !== "") {
      directories.add(parent);
      parent = parentPath(parent);
    }
  }
  return { files, directories };
}

// childPath joins a parent directory path and a leaf name (root parent = "").
export function childPath(parent: string, name: string): string {
  return parent ? `${parent}/${name}` : name;
}

// parentPath returns the containing directory of a path ("" for a top-level
// entry).
export function parentPath(path: string): string {
  const i = path.lastIndexOf("/");
  return i < 0 ? "" : path.slice(0, i);
}

// flattenTree walks the fetched levels depth-first from the root, emitting a row
// per visible entry. An expanded directory recurses into its fetched children;
// an expanded-but-not-yet-fetched directory simply shows no children (the view
// triggers the fetch). Directories precede files within a level (the sidecar
// already sorts each level trees-first, then by name).
export function flattenTree(
  children: ChildrenByPath,
  expanded: ReadonlySet<string>,
  root = "",
  depth = 0,
): FileRow[] {
  const entries = children[root];
  if (!entries) return [];
  const out: FileRow[] = [];
  for (const e of entries) {
    const path = childPath(root, e.name);
    const isDir = e.type === "tree";
    const isExpanded = isDir && expanded.has(path);
    out.push({ name: e.name, path, type: e.type, size: e.size, depth, expanded: isExpanded });
    if (isExpanded) {
      out.push(...flattenTree(children, expanded, path, depth + 1));
    }
  }
  return out;
}

// toggled returns a new expanded set with path added or removed.
export function toggled(expanded: ReadonlySet<string>, path: string): Set<string> {
  const next = new Set(expanded);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  return next;
}

// withChildren returns a new ChildrenByPath with path's fetched entries stored.
export function withChildren(
  children: ChildrenByPath,
  path: string,
  entries: TreeEntry[],
): ChildrenByPath {
  return { ...children, [path]: entries };
}

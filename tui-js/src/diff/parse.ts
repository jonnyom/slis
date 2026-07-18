// Pure unified-diff parser: turns a git patch string into per-file, per-hunk
// structures. No view code here — the diff pane (and the wave-2 side-by-side /
// syntax-highlighted differ) consume this. Keep it dependency-free and total.

export type DiffLineType = "add" | "del" | "context" | "meta";

/** High-level change status for a file, for the file-tree glyph (A/M/D/R). */
export type FileStatus = "added" | "deleted" | "modified" | "renamed";

export interface DiffLine {
  type: DiffLineType;
  content: string; // without the leading +/-/space marker
  /** 1-based line number in the old file (undefined for added lines). */
  oldNumber?: number;
  /** 1-based line number in the new file (undefined for deleted lines). */
  newNumber?: number;
}

export interface DiffHunk {
  header: string; // the raw @@ ... @@ line
  oldStart: number;
  newStart: number;
  lines: DiffLine[];
}

export interface FileDiff {
  /** New path (or old path for a deletion). */
  path: string;
  oldPath?: string; // set when the file was renamed
  binary: boolean;
  status: FileStatus;
  added: number; // added line count (0 for binary)
  deleted: number; // deleted line count (0 for binary)
  hunks: DiffHunk[];
}

const NEW_FILE: FileDiff["status"] = "modified";

/** The one-letter tree glyph for a file status. */
export function statusGlyph(status: FileStatus): string {
  switch (status) {
    case "added":
      return "A";
    case "deleted":
      return "D";
    case "renamed":
      return "R";
    case "modified":
    default:
      return "M";
  }
}

const HUNK_RE = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

function pathFromGitHeader(line: string): { old?: string; new?: string } {
  // "diff --git a/foo b/bar"
  const m = line.match(/^diff --git a\/(.+) b\/(.+)$/);
  if (!m) return {};
  return { old: m[1], new: m[2] };
}

function stripQuotes(p: string): string {
  return p.replace(/^"(.*)"$/, "$1");
}

/**
 * Parse a unified diff. Handles multiple files, renames, binary files, and the
 * `--- /dev/null` / `+++ /dev/null` add/delete markers. Robust to leading
 * `diff --git` blocks and to a bare single-file patch with no git header.
 */
export function parseUnifiedDiff(patch: string): FileDiff[] {
  const files: FileDiff[] = [];
  const lines = patch.split("\n");
  let current: FileDiff | null = null;
  let hunk: DiffHunk | null = null;
  let oldNo = 0;
  let newNo = 0;

  const pushFile = () => {
    if (current) files.push(current);
  };

  for (const raw of lines) {
    if (raw.startsWith("diff --git ")) {
      pushFile();
      const p = pathFromGitHeader(raw);
      current = {
        path: stripQuotes(p.new ?? p.old ?? "?"),
        binary: false,
        status: NEW_FILE,
        added: 0,
        deleted: 0,
        hunks: [],
      };
      hunk = null;
      continue;
    }
    if (!current) {
      // Bare patch without a git header — synthesize a file on the first ---.
      if (raw.startsWith("--- ")) {
        current = { path: "?", binary: false, status: NEW_FILE, added: 0, deleted: 0, hunks: [] };
        hunk = null;
      } else {
        continue;
      }
    }
    if (raw.startsWith("rename from ")) {
      current.oldPath = stripQuotes(raw.slice("rename from ".length));
      continue;
    }
    if (raw.startsWith("rename to ")) {
      current.path = stripQuotes(raw.slice("rename to ".length));
      current.status = "renamed";
      continue;
    }
    if (raw.startsWith("Binary files ") || raw.startsWith("GIT binary patch")) {
      current.binary = true;
      continue;
    }
    if (raw.startsWith("--- ")) {
      const p = raw.slice(4).trim();
      if (p === "/dev/null") {
        if (current.status !== "renamed") current.status = "added";
      } else if (current.path === "?") {
        current.path = stripQuotes(p.replace(/^a\//, ""));
      }
      continue;
    }
    if (raw.startsWith("+++ ")) {
      const p = raw.slice(4).trim();
      if (p === "/dev/null") {
        if (current.status !== "renamed") current.status = "deleted";
      } else {
        current.path = stripQuotes(p.replace(/^b\//, ""));
      }
      continue;
    }
    const hm = raw.match(HUNK_RE);
    if (hm) {
      oldNo = Number(hm[1]);
      newNo = Number(hm[2]);
      hunk = { header: raw, oldStart: oldNo, newStart: newNo, lines: [] };
      current.hunks.push(hunk);
      continue;
    }
    if (!hunk) continue; // index/mode lines between header and first hunk
    if (raw.startsWith("\\")) continue; // "\ No newline at end of file"
    if (raw.startsWith("+")) {
      hunk.lines.push({ type: "add", content: raw.slice(1), newNumber: newNo++ });
      current.added++;
    } else if (raw.startsWith("-")) {
      hunk.lines.push({ type: "del", content: raw.slice(1), oldNumber: oldNo++ });
      current.deleted++;
    } else {
      hunk.lines.push({
        type: "context",
        content: raw.startsWith(" ") ? raw.slice(1) : raw,
        oldNumber: oldNo++,
        newNumber: newNo++,
      });
    }
  }
  pushFile();
  return files;
}

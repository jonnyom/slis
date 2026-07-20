// Rich full-screen differ: a left file tree (path · +/- · A/M/D/R glyph) driving
// a right diff pane that renders unified OR side-by-side, with gutter line
// numbers, per-token syntax colour and word-level intra-line highlighting.
//
// Perf contract (see docs plan wave 2): the patch is parsed once per scope
// (memoized on the repos array) and the styled render model is built once per
// selected file (memoized on file identity + view mode). Scrolling and hunk
// jumps never re-tokenize — they only move the ScrollBox.

import { useKeyboard } from "@opentui/react";
import type { ScrollBoxRenderable } from "@opentui/core";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { DiffRepo, DiffScope, ReviewComment } from "../rpc/types";
import { parseUnifiedDiff, statusGlyph, type FileDiff } from "../diff/parse";
import { diffRangeComment, linesWithComments, type DiffSide } from "../review/context";
import { langForPath } from "../diff/tokenize";
import {
  buildFileRows,
  type Cell,
  type SbsSide,
  type SbsRowR,
  type UnifiedRow,
} from "../diff/rows";
import { color, colorForKind, diffColor, glyph, statusColor } from "../theme";
import { normalizeKeyName } from "../util/keys";
import { shortcutAction } from "../util/shortcut-contract";
import { BOLD, DIM } from "./ui";

export type DiffMode = "unified" | "split";

const SCOPE_LABEL: Record<DiffScope, string> = {
  working: "working tree (dirty)",
  parent: "vs parent",
  trunk: "vs trunk",
};

interface RepoGroup {
  repo: string;
  branch: string;
  err?: string;
  files: FileDiff[];
}

interface FlatFile {
  repo: string;
  branch: string;
  file: FileDiff;
}

// Two stable gutter columns: the visible line/range cursor, then the pending
// comment marker (✎). Blank cells preserve the diff's alignment.
function LineMark({ marked, cursor, selected }: { marked: boolean; cursor: boolean; selected: boolean }): ReactNode {
  return (
    <>
      <span fg={color.cursorBar} attributes={BOLD}>
        {cursor ? glyph.focusBar : selected ? "│" : " "}
      </span>
      <span fg={color.candidate} attributes={BOLD}>
        {marked ? glyph.comment : " "}
      </span>
    </>
  );
}

function gutter(n: number | undefined): string {
  return n === undefined ? "    " : String(n).padStart(4);
}

function markerColor(lineType: string): string {
  return lineType === "add"
    ? diffColor.add
    : lineType === "del"
      ? diffColor.del
      : color.dim;
}

function markerChar(lineType: string): string {
  return lineType === "add" ? "+" : lineType === "del" ? "-" : " ";
}

function CellSpans({ cells, lineType }: { cells: Cell[]; lineType: string }): ReactNode {
  if (cells.length === 0) return <span> </span>;
  const changeBg = lineType === "del" ? diffColor.delChangeBg : diffColor.addChangeBg;
  return (
    <>
      {cells.map((c, i) => (
        <span key={i} fg={colorForKind(c.kind)} bg={c.changed ? changeBg : undefined}>
          {c.text}
        </span>
      ))}
    </>
  );
}

function UnifiedLine({
  row,
  marked,
  cursor,
  selected,
  rowIndex,
}: {
  row: Extract<UnifiedRow, { kind: "line" }>;
  marked: boolean;
  cursor: boolean;
  selected: boolean;
  rowIndex: number;
}): ReactNode {
  return (
    <box
      id={`diffline-${rowIndex}`}
      flexDirection="row"
      backgroundColor={row.lineType === "add" ? diffColor.addBg : undefined}
    >
      <text flexShrink={0} wrapMode="none">
        <LineMark marked={marked} cursor={cursor} selected={selected} />
        <span fg={cursor || selected ? color.white : diffColor.gutter}>
          {gutter(row.oldNumber)} {gutter(row.newNumber)}{" "}
        </span>
        <span fg={markerColor(row.lineType)} attributes={BOLD}>
          {markerChar(row.lineType)}{" "}
        </span>
      </text>
      <text flexGrow={1} flexShrink={1} wrapMode="char">
        <CellSpans cells={row.cells} lineType={row.lineType} />
      </text>
    </box>
  );
}

function SbsCell({
  side,
  half,
  marked,
  cursor,
  selected,
}: {
  side: SbsSide;
  half: number;
  marked: boolean;
  cursor: boolean;
  selected: boolean;
}): ReactNode {
  const isBlank = side.lineType === "blank";
  const g = side.lineType === "add" ? gutter(side.newNumber) : gutter(side.oldNumber);
  return (
    <box
      width={half}
      overflow="hidden"
      flexShrink={0}
      flexDirection="row"
      backgroundColor={side.lineType === "add" ? diffColor.addBg : undefined}
    >
      {isBlank ? (
        <text wrapMode="none">
          <span fg={color.dim}> </span>
        </text>
      ) : (
        <>
          <text flexShrink={0} wrapMode="none">
            <LineMark marked={marked} cursor={cursor} selected={selected} />
            <span fg={cursor || selected ? color.white : diffColor.gutter}>{g} </span>
            <span fg={markerColor(side.lineType)} attributes={BOLD}>
              {markerChar(side.lineType)}{" "}
            </span>
          </text>
          <text flexGrow={1} flexShrink={1} wrapMode="char">
            <CellSpans cells={side.cells} lineType={side.lineType} />
          </text>
        </>
      )}
    </box>
  );
}

function SbsLine({
  row,
  half,
  markedOld,
  markedNew,
  cursorSide,
  selectedOld,
  selectedNew,
  rowIndex,
}: {
  row: Extract<SbsRowR, { kind: "line" }>;
  half: number;
  markedOld: boolean;
  markedNew: boolean;
  cursorSide?: DiffSide;
  selectedOld: boolean;
  selectedNew: boolean;
  rowIndex: number;
}): ReactNode {
  return (
    <box id={`diffline-${rowIndex}`} flexDirection="row">
      <SbsCell
        side={row.left}
        half={half}
        marked={markedOld}
        cursor={cursorSide === "old"}
        selected={selectedOld}
      />
      <box width={1} flexShrink={0}>
        <text fg={color.border}>│</text>
      </box>
      <SbsCell
        side={row.right}
        half={half}
        marked={markedNew}
        cursor={cursorSide === "new"}
        selected={selectedNew}
      />
    </box>
  );
}

// ── diff view ──────────────────────────────────────────────────────────────────

// The context DiffView hands up when the user comments on the selected hunk. The
// caller (cockpit) adds the slice and opens the composer overlay.
export interface DiffCommentTarget {
  repo: string;
  branch: string;
  file: string;
  line: number;
  endLine?: number;
  side: DiffSide;
  hunk: string;
}

export interface DiffViewProps {
  enabled: boolean;
  repos: DiffRepo[];
  scope: DiffScope;
  mode: DiffMode;
  width: number;
  height: number;
  // Pending review comments for the slice (F2) — drives the ✎ gutter markers.
  comments: ReviewComment[];
  onCycleScope: () => void;
  onToggleMode: () => void;
  onClose: () => void;
  onQuit: () => void;
  onAttach: () => void;
  onLaunchAgent: () => void;
  onConfigureAgents: () => void;
  // c → comment on the selected line/range; V → open pending review.
  onComment: (target: DiffCommentTarget) => void;
  onReview: () => void;
}

export function DiffView(props: DiffViewProps): ReactNode {
  const { repos, scope, mode, enabled } = props;
  const scrollRef = useRef<ScrollBoxRenderable>(null);
  const fileScrollRef = useRef<ScrollBoxRenderable>(null);
  const [fileSel, setFileSel] = useState(0);
  const [pane, setPane] = useState<"files" | "diff">("files");
  const [lineSel, setLineSel] = useState(0);
  const [rangeAnchor, setRangeAnchor] = useState<number | null>(null);
  const [diffSide, setDiffSide] = useState<DiffSide>("new");

  // Parse every repo's patch once per scope refetch (memoized on `repos`).
  const groups: RepoGroup[] = useMemo(
    () =>
      repos.map((r) => ({
        repo: r.repo,
        branch: r.branch,
        err: r.err,
        files: r.patch ? parseUnifiedDiff(r.patch) : [],
      })),
    [repos],
  );

  const flat: FlatFile[] = useMemo(() => {
    const out: FlatFile[] = [];
    for (const g of groups)
      for (const file of g.files) out.push({ repo: g.repo, branch: g.branch, file });
    return out;
  }, [groups]);

  // Clamp selection when the file set shrinks (e.g. after a scope change).
  useEffect(() => {
    setFileSel((i) => Math.max(0, Math.min(i, Math.max(0, flat.length - 1))));
  }, [flat.length]);

  const selected = flat[fileSel];

  // Build the styled render model once per selected file + mode.
  const built = useMemo(
    () => (selected ? buildFileRows(selected.file, langForPath(selected.file.path)) : null),
    [selected],
  );

  const hunkOffsets = mode === "split" ? built?.sbsHunkOffsets : built?.unifiedHunkOffsets;
  const hunkCount = hunkOffsets?.length ?? 0;

  type ReviewableLine = {
    renderIndex: number;
    hunkIndex: number;
    line: number;
    side: DiffSide;
  };

  // Split view exposes two independent review surfaces. Unified view derives
  // the side from each row: deletion rows anchor old-file lines; additions and
  // context rows anchor new-file lines.
  const reviewableBySide = useMemo(() => {
    const old: ReviewableLine[] = [];
    const newLines: ReviewableLine[] = [];
    for (const [renderIndex, row] of (built?.sideBySide ?? []).entries()) {
      if (row.kind !== "line") continue;
      if (row.left.oldNumber !== undefined)
        old.push({ renderIndex, hunkIndex: row.hunkIndex, line: row.left.oldNumber, side: "old" });
      if (row.right.newNumber !== undefined)
        newLines.push({ renderIndex, hunkIndex: row.hunkIndex, line: row.right.newNumber, side: "new" });
    }
    return { old, new: newLines };
  }, [built]);

  const unifiedReviewable = useMemo(() => {
    const out: ReviewableLine[] = [];
    for (const [renderIndex, row] of (built?.unified ?? []).entries()) {
      if (row.kind !== "line") continue;
      if (row.lineType === "del" && row.oldNumber !== undefined)
        out.push({ renderIndex, hunkIndex: row.hunkIndex, line: row.oldNumber, side: "old" });
      else if (row.newNumber !== undefined)
        out.push({ renderIndex, hunkIndex: row.hunkIndex, line: row.newNumber, side: "new" });
    }
    return out;
  }, [built]);

  const reviewable = mode === "split" ? reviewableBySide[diffSide] : unifiedReviewable;

  const activeLine = reviewable[lineSel];
  const selectedRenderRows = useMemo(() => {
    const out = new Set<number>();
    if (!activeLine) return out;
    const from = rangeAnchor === null ? lineSel : Math.min(rangeAnchor, lineSel);
    const to = rangeAnchor === null ? lineSel : Math.max(rangeAnchor, lineSel);
    for (let i = from; i <= to; i++) {
      const row = reviewable[i];
      if (row?.hunkIndex === activeLine.hunkIndex && row.side === activeLine.side)
        out.add(row.renderIndex);
    }
    return out;
  }, [activeLine, lineSel, rangeAnchor, reviewable]);

  const markedOldLines = useMemo(
    () => (selected ? linesWithComments(props.comments, selected.repo, selected.file.path, "old") : new Set<number>()),
    [props.comments, selected],
  );
  const markedNewLines = useMemo(
    () => (selected ? linesWithComments(props.comments, selected.repo, selected.file.path, "new") : new Set<number>()),
    [props.comments, selected],
  );

  // Compose a comment target from the exact selected new-file line range.
  const commentOnSelection = () => {
    if (!selected || !activeLine) return;
    const anchor = rangeAnchor === null ? activeLine : reviewable[rangeAnchor];
    if (!anchor) return;
    const hc = diffRangeComment(
      selected.file,
      activeLine.hunkIndex,
      anchor.line,
      activeLine.line,
      activeLine.side,
    );
    if (!hc) return;
    props.onComment({
      repo: selected.repo,
      branch: selected.branch,
      file: selected.file.path,
      line: hc.line,
      endLine: hc.endLine,
      side: activeLine.side,
      hunk: hc.hunk,
    });
    // The range has been consumed by the composer. Return to normal line
    // navigation immediately so adding (or cancelling) a comment never leaves
    // the diff stuck in a temporary selection mode.
    setRangeAnchor(null);
  };

  // Reset the visible line cursor whenever the selected file or mode changes.
  const fileKey = selected ? `${selected.repo}:${selected.file.path}` : "";
  useEffect(() => {
    scrollRef.current?.scrollTo(0);
    setLineSel(0);
    setRangeAnchor(null);
    setDiffSide("new");
  }, [fileKey, mode]);

  // Keep the selected file row visible in the file list.
  useEffect(() => {
    fileScrollRef.current?.scrollChildIntoView(`file-${fileSel}`);
  }, [fileSel]);

  const moveLine = (delta: number) => {
    if (reviewable.length === 0) return;
    setLineSel((current) => {
      let next = Math.max(0, Math.min(reviewable.length - 1, current + delta));
      if (rangeAnchor !== null) {
        const hunk = reviewable[rangeAnchor]?.hunkIndex;
        const side = reviewable[rangeAnchor]?.side;
        while (
          next !== current &&
          (reviewable[next]?.hunkIndex !== hunk || reviewable[next]?.side !== side)
        )
          next -= Math.sign(delta);
      }
      const row = reviewable[next];
      if (row) scrollRef.current?.scrollChildIntoView(`diffline-${row.renderIndex}`);
      return next;
    });
  };

  const switchDiffSide = (side: DiffSide) => {
    if (mode !== "split" || side === diffSide) return;
    const nextRows = reviewableBySide[side];
    const renderIndex = activeLine?.renderIndex ?? 0;
    let next = nextRows.findIndex((row) => row.renderIndex === renderIndex);
    if (next < 0 && nextRows.length > 0) {
      next = nextRows.reduce(
        (best, row, i) =>
          Math.abs(row.renderIndex - renderIndex) <
          Math.abs(nextRows[best]!.renderIndex - renderIndex)
            ? i
            : best,
        0,
      );
    }
    setDiffSide(side);
    setLineSel(Math.max(0, next));
    setRangeAnchor(null);
    const row = nextRows[Math.max(0, next)];
    if (row) scrollRef.current?.scrollChildIntoView(`diffline-${row.renderIndex}`);
  };

  const jumpHunk = (delta: number) => {
    if (!activeLine || reviewable.length === 0) return;
    const hunks = [...new Set(reviewable.map((r) => r.hunkIndex))];
    const at = Math.max(0, hunks.indexOf(activeLine.hunkIndex));
    const target = hunks[Math.max(0, Math.min(hunks.length - 1, at + delta))];
    const next = reviewable.findIndex((r) => r.hunkIndex === target);
    if (next >= 0) {
      setLineSel(next);
      setRangeAnchor(null);
      scrollRef.current?.scrollChildIntoView(`diffline-${reviewable[next]!.renderIndex}`);
    }
  };

  useKeyboard((key) => {
    if (!enabled) return;
    const name = normalizeKeyName(key);
    const shortcut = shortcutAction("diff", name);
    if (shortcut === "configure-agents") return props.onConfigureAgents();
    if (name === "q") return props.onQuit();
    if (shortcut === "attach-agent") return props.onAttach();
    if (shortcut === "launch-agent") return props.onLaunchAgent();
    if (pane === "diff" && mode === "split" && (name === "h" || name === "left")) {
      switchDiffSide("old");
      return;
    }
    if (pane === "diff" && mode === "split" && (name === "l" || name === "right")) {
      switchDiffSide("new");
      return;
    }
    if (name === "escape" || (name === "h" && mode !== "split")) {
      if (pane === "diff") {
        setPane("files");
        setRangeAnchor(null);
        return;
      }
      return props.onClose();
    }
    if (name === "tab") {
      setPane((p) => (p === "files" ? "diff" : "files"));
      setRangeAnchor(null);
      return;
    }
    if (name === "c") {
      if (pane === "files") return setPane("diff");
      return commentOnSelection();
    }
    if (shortcut === "pending-review") return props.onReview();
    if (name === "t") return props.onToggleMode();
    if (name === "b") return props.onCycleScope();
    if (name === "j" || name === "down")
      return pane === "files"
        ? setFileSel((i) => Math.min(flat.length - 1, i + 1))
        : moveLine(1);
    if (name === "k" || name === "up")
      return pane === "files" ? setFileSel((i) => Math.max(0, i - 1)) : moveLine(-1);
    if (pane === "diff" && (name === "v" || name === "space")) {
      setRangeAnchor((anchor) => (anchor === null ? lineSel : null));
      return;
    }
    if (name === "]" || name === "n") return jumpHunk(1);
    if (name === "[" || name === "p") return jumpHunk(-1);
    // enter/l moves focus from the file list into its diff lines.
    if (name === "return" || name === "enter" || name === "l" || name === "right") {
      setPane("diff");
      const row = reviewable[lineSel];
      if (row) scrollRef.current?.scrollChildIntoView(`diffline-${row.renderIndex}`);
      return;
    }
    if (name === "left") return scrollRef.current?.scrollBy({ x: -8, y: 0 });
    if ((name === "g" || name === "G") && pane === "diff") {
      const next = name === "g" ? 0 : Math.max(0, reviewable.length - 1);
      setLineSel(next);
      setRangeAnchor(null);
      const row = reviewable[next];
      if (row) scrollRef.current?.scrollChildIntoView(`diffline-${row.renderIndex}`);
      return;
    }
    if (key.ctrl && name === "d") return scrollRef.current?.scrollBy(10);
    if (key.ctrl && name === "u") return scrollRef.current?.scrollBy(-10);
    if (name === "pagedown") return scrollRef.current?.scrollBy(15);
    if (name === "pageup") return scrollRef.current?.scrollBy(-15);
  });

  const listW = Math.max(24, Math.min(46, Math.floor(props.width * 0.32)));
  const diffW = props.width - listW;
  const half = Math.max(8, Math.floor((diffW - 5) / 2));
  // Header + footer each take exactly one row; both panels are pinned to the
  // remainder so their bottom rules line up and never reach the footer (P3).
  const bodyH = Math.max(1, props.height - 2);

  const totalAdded = flat.reduce((a, f) => a + Math.max(f.file.added, 0), 0);
  const totalDeleted = flat.reduce((a, f) => a + Math.max(f.file.deleted, 0), 0);

  const rightTitle = selected
    ? `${selected.repo} · ${selected.file.path}`
    : "no file";

  return (
    <box flexDirection="column" width="100%" height="100%">
      {/* header */}
      <box flexDirection="row" justifyContent="space-between">
        <text wrapMode="none">
          <span fg={color.title} attributes={BOLD}>
            Diff
          </span>
          <span fg={color.dim}>
            {"  "}
            {flat.length} files{" "}
          </span>
          <span fg={diffColor.add}>+{totalAdded}</span>
          <span fg={diffColor.del}> -{totalDeleted}</span>
          <span fg={color.dim}> · {SCOPE_LABEL[scope]} · </span>
          <span fg={color.candidate}>{mode === "split" ? "side-by-side" : "unified"}</span>
        </text>
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          {pane === "files" ? "[enter/tab] select lines" : "[v/space] toggle range  [c] comment"}  [V] review  [a] attach  [C] launch  [esc] back
        </text>
      </box>

      {/* body — pinned to an explicit height (header + footer reserved) and
          clipped so the file panel's bottom rule can never bleed into the
          footer row (P3) */}
      <box flexDirection="row" height={bodyH} overflow="hidden">
        <box width={listW} flexShrink={0} flexDirection="column">
          {/* file list gets its own scroll ref via a nested scrollbox */}
          <FileListScroller
            groups={groups}
            flat={flat}
            sel={fileSel}
            width={listW}
            height={bodyH}
            scrollRef={fileScrollRef}
            focused={pane === "files"}
          />
        </box>
        <box flexGrow={1} flexDirection="column">
          <box
            border
            borderStyle="rounded"
            borderColor={pane === "diff" ? color.borderFocus : color.border}
            title={rightTitle}
            titleColor={pane === "diff" ? color.borderFocus : color.dim}
            height={bodyH}
            paddingLeft={1}
            paddingRight={1}
            overflow="hidden"
          >
            <scrollbox
              ref={scrollRef}
              flexGrow={1}
              verticalScrollbarOptions={{ visible: true }}
              horizontalScrollbarOptions={{ visible: false }}
              viewportCulling
            >
              {!selected || !built ? (
                <text fg={color.dim} attributes={DIM}>
                  {flat.length === 0 ? "no changes in this scope" : "select a file"}
                </text>
              ) : mode === "split" ? (
                built.sideBySide.map((row, i) =>
                  row.kind === "hunk" ? (
                    <text key={i} fg={diffColor.hunk} wrapMode="none">
                      {"   " + row.header}
                    </text>
                  ) : (
                    <SbsLine
                      key={i}
                      row={row}
                      half={half}
                      markedOld={markedOldLines.has(row.left.oldNumber ?? -1)}
                      markedNew={markedNewLines.has(row.right.newNumber ?? -1)}
                      cursorSide={
                        pane === "diff" && activeLine?.renderIndex === i
                          ? activeLine.side
                          : undefined
                      }
                      selectedOld={
                        pane === "diff" && diffSide === "old" && selectedRenderRows.has(i)
                      }
                      selectedNew={
                        pane === "diff" && diffSide === "new" && selectedRenderRows.has(i)
                      }
                      rowIndex={i}
                    />
                  ),
                )
              ) : (
                built.unified.map((row, i) =>
                  row.kind === "hunk" ? (
                    <text key={i} fg={diffColor.hunk} wrapMode="none">
                      {"  " + row.header}
                    </text>
                  ) : (
                    <UnifiedLine
                      key={i}
                      row={row}
                      marked={
                        row.lineType === "del"
                          ? markedOldLines.has(row.oldNumber ?? -1)
                          : markedNewLines.has(row.newNumber ?? -1)
                      }
                      cursor={pane === "diff" && activeLine?.renderIndex === i}
                      selected={pane === "diff" && selectedRenderRows.has(i)}
                      rowIndex={i}
                    />
                  ),
                )
              )}
            </scrollbox>
          </box>
        </box>
      </box>

      {/* footer — reserve exactly one row so a long unwrapped hint never bleeds
          into the file panel's bottom rule (P3) */}
      <box width="100%" height={1} overflow="hidden">
        <text wrapMode="none" fg={color.dim} attributes={DIM}>
          {pane === "files"
            ? "j/k file · enter/tab review lines · c start comment"
            : `j/k ${activeLine?.side ?? diffSide} line${activeLine ? ` ${activeLine.line}` : ""}${mode === "split" ? " · h/l old/new side" : ""} · v/space range · c comment · n/p hunk`}
          {hunkCount ? ` (${hunkCount})` : ""} · V review · a attach · C launch · t unified/split · b scope · esc back
        </text>
      </box>
    </box>
  );
}

// FileList needs its own scrollbox ref; wrap it so the ref reaches the inner
// scrollbox without threading it through FileList's JSX above.
function FileListScroller({
  groups,
  flat,
  sel,
  width,
  height,
  scrollRef,
  focused,
}: {
  groups: RepoGroup[];
  flat: FlatFile[];
  sel: number;
  width: number;
  height: number;
  scrollRef: React.RefObject<ScrollBoxRenderable | null>;
  focused: boolean;
}): ReactNode {
  return (
    <box
      border
      borderStyle="rounded"
      borderColor={focused ? color.borderFocus : color.border}
      title="Files"
      titleColor={focused ? color.borderFocus : color.dim}
      width={width}
      height={height}
      paddingLeft={1}
      paddingRight={1}
      overflow="hidden"
      flexDirection="column"
    >
      <scrollbox
        ref={scrollRef}
        flexGrow={1}
        verticalScrollbarOptions={{ visible: true }}
        horizontalScrollbarOptions={{ visible: false }}
      >
        {groups.map((g) => (
          <box key={g.repo} flexDirection="column">
            <text wrapMode="none">
              <span fg={color.repoHeader} attributes={BOLD}>
                {g.repo}
              </span>
              <span fg={color.dim}> {g.branch}</span>
            </text>
            {g.err ? (
              <text fg={color.missing} wrapMode="none">
                {"  "}
                {g.err}
              </text>
            ) : g.files.length === 0 ? (
              <text fg={color.dim} attributes={DIM} wrapMode="none">
                {"  no changes"}
              </text>
            ) : (
              g.files.map((f) => {
                const idx = flat.findIndex((x) => x.file === f);
                const selected = idx === sel;
                return (
                  <text key={f.path} id={`file-${idx}`} wrapMode="none">
                    <span fg={color.cursorBar}>
                      {selected && focused ? glyph.focusBar : " "}
                    </span>
                    <span fg={statusColor(f.status)} attributes={BOLD}>
                      {statusGlyph(f.status)}{" "}
                    </span>
                    <span fg={selected ? color.white : color.fg}>{f.path}</span>
                    {f.binary ? (
                      <span fg={color.dim}> bin</span>
                    ) : (
                      <>
                        <span fg={diffColor.add}> +{f.added}</span>
                        <span fg={diffColor.del}> -{f.deleted}</span>
                      </>
                    )}
                  </text>
                );
              })
            )}
          </box>
        ))}
      </scrollbox>
    </box>
  );
}

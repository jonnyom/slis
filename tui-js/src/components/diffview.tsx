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
import type { DiffRepo, DiffScope } from "../rpc/types";
import { parseUnifiedDiff, statusGlyph, type FileDiff } from "../diff/parse";
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
  file: FileDiff;
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

function UnifiedLine({ row }: { row: Extract<UnifiedRow, { kind: "line" }> }): ReactNode {
  return (
    <text wrapMode="none">
      <span fg={diffColor.gutter}>
        {gutter(row.oldNumber)} {gutter(row.newNumber)}{" "}
      </span>
      <span fg={markerColor(row.lineType)} attributes={BOLD}>
        {markerChar(row.lineType)}{" "}
      </span>
      <CellSpans cells={row.cells} lineType={row.lineType} />
    </text>
  );
}

function SbsCell({ side, half }: { side: SbsSide; half: number }): ReactNode {
  const isBlank = side.lineType === "blank";
  const g = side.lineType === "add" ? gutter(side.newNumber) : gutter(side.oldNumber);
  return (
    <box width={half} overflow="hidden" flexShrink={0}>
      <text wrapMode="none">
        {isBlank ? (
          <span fg={color.dim}> </span>
        ) : (
          <>
            <span fg={diffColor.gutter}>{g} </span>
            <span fg={markerColor(side.lineType)} attributes={BOLD}>
              {markerChar(side.lineType)}{" "}
            </span>
            <CellSpans cells={side.cells} lineType={side.lineType} />
          </>
        )}
      </text>
    </box>
  );
}

function SbsLine({ row, half }: { row: Extract<SbsRowR, { kind: "line" }>; half: number }): ReactNode {
  return (
    <box flexDirection="row">
      <SbsCell side={row.left} half={half} />
      <box width={1} flexShrink={0}>
        <text fg={color.border}>│</text>
      </box>
      <SbsCell side={row.right} half={half} />
    </box>
  );
}

// ── diff view ──────────────────────────────────────────────────────────────────

export interface DiffViewProps {
  enabled: boolean;
  repos: DiffRepo[];
  scope: DiffScope;
  mode: DiffMode;
  width: number;
  height: number;
  onCycleScope: () => void;
  onToggleMode: () => void;
  onClose: () => void;
  onQuit: () => void;
}

export function DiffView(props: DiffViewProps): ReactNode {
  const { repos, scope, mode, enabled } = props;
  const scrollRef = useRef<ScrollBoxRenderable>(null);
  const fileScrollRef = useRef<ScrollBoxRenderable>(null);
  const [fileSel, setFileSel] = useState(0);
  const [hunkSel, setHunkSel] = useState(0);

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
    for (const g of groups) for (const file of g.files) out.push({ repo: g.repo, file });
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

  // Reset scroll + hunk cursor whenever the selected file or view mode changes.
  const fileKey = selected ? `${selected.repo}:${selected.file.path}` : "";
  useEffect(() => {
    scrollRef.current?.scrollTo(0);
    setHunkSel(0);
  }, [fileKey, mode]);

  // Keep the selected file row visible in the file list.
  useEffect(() => {
    fileScrollRef.current?.scrollChildIntoView(`file-${fileSel}`);
  }, [fileSel]);

  const jumpHunk = (delta: number) => {
    if (!hunkOffsets || hunkOffsets.length === 0) return;
    const next = Math.max(0, Math.min(hunkOffsets.length - 1, hunkSel + delta));
    setHunkSel(next);
    scrollRef.current?.scrollTo(hunkOffsets[next]!);
  };

  useKeyboard((key) => {
    if (!enabled) return;
    const name = normalizeKeyName(key);
    if (name === "q") return props.onQuit();
    if (name === "escape" || name === "h") return props.onClose();
    if (name === "t") return props.onToggleMode();
    if (name === "b") return props.onCycleScope();
    if (name === "j" || name === "down")
      return setFileSel((i) => Math.min(flat.length - 1, i + 1));
    if (name === "k" || name === "up")
      return setFileSel((i) => Math.max(0, i - 1));
    if (name === "]" || name === "n") return jumpHunk(1);
    if (name === "[" || name === "p") return jumpHunk(-1);
    if (name === "return" || name === "enter" || name === "l" || name === "right")
      return scrollRef.current?.scrollTo(0);
    if (name === "left") return scrollRef.current?.scrollBy({ x: -8, y: 0 });
    if (name === "g") return scrollRef.current?.scrollTo(0);
    if (name === "G") return scrollRef.current?.scrollTo(scrollRef.current.scrollHeight);
    if (key.ctrl && name === "d") return scrollRef.current?.scrollBy(10);
    if (key.ctrl && name === "u") return scrollRef.current?.scrollBy(-10);
    if (name === "pagedown") return scrollRef.current?.scrollBy(15);
    if (name === "pageup") return scrollRef.current?.scrollBy(-15);
  });

  const listW = Math.max(24, Math.min(46, Math.floor(props.width * 0.32)));
  const diffW = props.width - listW;
  const half = Math.max(8, Math.floor((diffW - 5) / 2));

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
          [esc] back  [t] view  [b] scope
        </text>
      </box>

      {/* body */}
      <box flexDirection="row" flexGrow={1}>
        <box width={listW} flexShrink={0} flexDirection="column">
          {/* file list gets its own scroll ref via a nested scrollbox */}
          <FileListScroller
            groups={groups}
            flat={flat}
            sel={fileSel}
            width={listW}
            scrollRef={fileScrollRef}
          />
        </box>
        <box flexGrow={1} flexDirection="column">
          <box
            border
            borderStyle="rounded"
            borderColor={color.borderFocus}
            title={rightTitle}
            titleColor={color.borderFocus}
            flexGrow={1}
            paddingLeft={1}
            paddingRight={1}
            overflow="hidden"
          >
            <scrollbox
              ref={scrollRef}
              flexGrow={1}
              scrollbarOptions={{ visible: true }}
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
                      {row.header}
                    </text>
                  ) : (
                    <SbsLine key={i} row={row} half={half} />
                  ),
                )
              ) : (
                built.unified.map((row, i) =>
                  row.kind === "hunk" ? (
                    <text key={i} fg={diffColor.hunk} wrapMode="none">
                      {row.header}
                    </text>
                  ) : (
                    <UnifiedLine key={i} row={row} />
                  ),
                )
              )}
            </scrollbox>
          </box>
        </box>
      </box>

      {/* footer */}
      <text wrapMode="none" fg={color.dim} attributes={DIM}>
        j/k file · [ ] / n p hunk{hunkCount ? ` (${hunkSel + 1}/${hunkCount})` : ""} · t
        unified/split · b scope · ^d/^u scroll · g/G top/bottom · esc back
      </text>
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
  scrollRef,
}: {
  groups: RepoGroup[];
  flat: FlatFile[];
  sel: number;
  width: number;
  scrollRef: React.RefObject<ScrollBoxRenderable | null>;
}): ReactNode {
  return (
    <box
      border
      borderStyle="rounded"
      borderColor={color.borderFocus}
      title="Files"
      titleColor={color.borderFocus}
      width={width}
      flexGrow={1}
      paddingLeft={1}
      paddingRight={1}
      overflow="hidden"
      flexDirection="column"
    >
      <scrollbox ref={scrollRef} flexGrow={1} scrollbarOptions={{ visible: true }}>
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
                    <span fg={color.cursorBar}>{selected ? glyph.focusBar : " "}</span>
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

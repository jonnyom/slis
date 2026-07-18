// Browser view: the top-level slice picker.
//
//   ┌ pulse bar ────────────────────────────────────────────┐
//   │ States rail (filters 1-8) │ preview (repos/PR/overview │
//   │ Slices list               │  · session tail · diff)    │
//   └ footer hints ─────────────────────────────────────────┘

import { useKeyboard } from "@opentui/react";
import { memo, useEffect, useMemo, useState, type ReactNode } from "react";
import type {
  CaptureResult,
  ConflictsResult,
  DiffResult,
  LsResult,
  PrStackEntry,
  RpcClient,
} from "../rpc/types";
import {
  attentionRank,
  FILTERS,
  needsRestack,
  workState,
  type SliceView,
} from "../state/derive";
import { color, glyph } from "../theme";
import { Panel } from "../components/panel";
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";

export interface BrowserProps {
  enabled: boolean;
  client: RpcClient;
  version: string;
  workspaceRoot: string;
  views: SliceView[];
  ls: LsResult;
  conflicts: ConflictsResult | null;
  width: number;
  height: number;
  onEnter: (slice: string) => void;
  onSwap: (slice: string) => void;
  onOpenTerm: (slice: string, launchAgent: boolean) => void;
  onRefresh: () => void;
  onToggleHelp: () => void;
  onToggleProcs: () => void;
  onQuit: () => void;
}

function glyphFor(view: SliceView): { g: string; c: string; bold: boolean } {
  const ws = workState(view);
  if (ws === "needs-you") {
    if (view.status === "waiting-input")
      return { g: glyph.waiting, c: color.wait, bold: true };
    if (view.status === "done") return { g: glyph.done, c: color.done, bold: true };
    return { g: glyph.inReview, c: color.missing, bold: true }; // changes requested
  }
  if (ws === "ready") return { g: glyph.ready, c: color.ready, bold: true };
  if (ws === "in-review") return { g: glyph.inReview, c: color.synced, bold: false };
  if (view.slice.active) return { g: glyph.live, c: color.live, bold: true };
  if (view.status === "running") return { g: glyph.running, c: color.fg, bold: false };
  return { g: glyph.idle, c: color.dim, bold: false };
}

const SliceRow = memo(function SliceRow({
  view,
  focused,
  listFocused,
}: {
  view: SliceView;
  focused: boolean;
  listFocused: boolean;
}): ReactNode {
  const { g, c, bold } = glyphFor(view);
  const marker = focused && listFocused ? glyph.focusBar : " ";
  const nameColor = focused ? color.white : color.fg;
  return (
    <text wrapMode="none">
      <span fg={color.cursorBar}>{marker}</span>
      <span fg={c} attributes={bold ? BOLD : 0}>
        {g}
      </span>
      <span fg={nameColor} attributes={focused ? BOLD : 0}>
        {" "}
        {view.slice.name}
      </span>
      {view.slice.active ? <span fg={color.live}> ●</span> : null}
      {view.slice.stale ? <span fg={color.wait}> ⚠</span> : null}
    </text>
  );
});

function PulseBar({
  count,
  views,
  version,
}: {
  count: number;
  views: SliceView[];
  version: string;
}): ReactNode {
  const active = views.find((v) => v.slice.active);
  const needYou = views.filter(
    (v) => v.status === "waiting-input" || v.status === "done",
  ).length;
  const ready = views.filter((v) => workState(v) === "ready").length;
  const restack = views.filter((v) => needsRestack(v)).length;
  return (
    <text wrapMode="none">
      <span fg={color.title} attributes={BOLD}>
        slis
      </span>
      <span fg={color.dim}> · {count} slices</span>
      {active ? (
        <span fg={color.live}>
          {"  "}
          {glyph.live} live: {active.slice.name}
        </span>
      ) : null}
      {needYou > 0 ? (
        <span fg={color.wait}>
          {"  "}
          {glyph.waiting} {needYou} need you
        </span>
      ) : null}
      {ready > 0 ? (
        <span fg={color.ready}>
          {"  "}
          {glyph.ready} {ready} ready
        </span>
      ) : null}
      {restack > 0 ? (
        <span fg={color.restack}>
          {"  "}
          {glyph.restack} {restack} need restack
        </span>
      ) : null}
      <span fg={color.dim}>  v{version}</span>
    </text>
  );
}

function StatesRail({
  views,
  filterIndex,
  focused,
  height,
}: {
  views: SliceView[];
  filterIndex: number;
  focused: boolean;
  height: number;
}): ReactNode {
  return (
    <Panel title="States" focused={focused} height={height}>
      {FILTERS.map((f, i) => {
        const n = views.filter(f.match).length;
        const active = i === filterIndex;
        const label = f.label.padEnd(13);
        return (
          <text key={f.key} wrapMode="none" attributes={active ? BOLD : 0}>
            <span fg={active ? color.title : color.dim}>
              {active ? glyph.filterMarker + " " : "  "}
            </span>
            <span fg={active ? color.white : color.fg}>{label}</span>
            <span fg={color.dim}>{String(n).padStart(2)}</span>
          </text>
        );
      })}
    </Panel>
  );
}

function repoPrLine(prs: PrStackEntry[] | undefined, repo: string): ReactNode {
  const pr = prs?.find((p) => p.repo === repo);
  if (!pr || pr.number === undefined) return <span fg={color.dim}> (no PR)</span>;
  const stateColor =
    pr.state === "MERGED"
      ? color.merged
      : pr.state === "OPEN"
        ? color.synced
        : color.dim;
  const review =
    pr.review_decision === "APPROVED"
      ? { t: " ✓ approved", c: color.synced }
      : pr.review_decision === "CHANGES_REQUESTED"
        ? { t: " ✗ changes", c: color.missing }
        : { t: "", c: color.dim };
  return (
    <span>
      <span fg={color.dim}> #{pr.number} </span>
      <span fg={stateColor}>{(pr.state ?? "").toLowerCase()}</span>
      {review.t ? <span fg={review.c}>{review.t}</span> : null}
    </span>
  );
}

function Preview({
  client,
  view,
  conflicts,
  height,
}: {
  client: RpcClient;
  view: SliceView | undefined;
  conflicts: ConflictsResult | null;
  height: number;
}): ReactNode {
  const [capture, setCapture] = useState<CaptureResult | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const slice = view?.slice.name;

  useEffect(() => {
    if (!slice) return;
    let live = true;
    setCapture(null);
    setDiff(null);
    client.capture({ slice, lines: 8 }).then((r) => live && setCapture(r), () => {});
    client
      .diff({ slice, scope: "working", format: "stat" })
      .then((r) => live && setDiff(r), () => {});
    return () => {
      live = false;
    };
  }, [client, slice]);

  if (!view) {
    return (
      <Panel title="Preview" flexGrow={1}>
        <text fg={color.dim} attributes={DIM}>
          No slice selected.
        </text>
      </Panel>
    );
  }

  const overlaps = (conflicts?.overlaps ?? []).filter((o) =>
    o.slices.includes(view.slice.name),
  );

  return (
    <Panel title={view.slice.name} flexGrow={1} height={height}>
      {/* Tags */}
      <text wrapMode="none">
        {view.slice.active ? <span fg={color.live}>{glyph.live} live  </span> : null}
        {view.slice.stale ? (
          <span fg={color.wait}>⚠ primary behind tip  </span>
        ) : null}
        {workState(view) === "ready" ? (
          <span fg={color.ready}>{glyph.ready} ready to clear  </span>
        ) : null}
        {view.status === "waiting-input" ? (
          <span fg={color.wait}>{glyph.waiting} needs you  </span>
        ) : null}
        {overlaps.length > 0 ? (
          <span fg={color.wait}>⚠ overlaps {overlaps.length}</span>
        ) : null}
        {!view.slice.active &&
        !view.slice.stale &&
        view.status !== "waiting-input" &&
        overlaps.length === 0 ? (
          <span fg={color.dim}>idle</span>
        ) : null}
      </text>
      <text> </text>
      {/* Per-repo branch + PR */}
      {view.slice.members.map((m) => (
        <text key={m.repo} wrapMode="none">
          <span fg={color.repoHeader} attributes={BOLD}>
            {m.repo}
          </span>
          <span fg={color.fg}>  {m.branch}</span>
          {repoPrLine(view.prs, m.repo)}
        </text>
      ))}
      <text> </text>
      {/* Diff stat */}
      {diff && diff.repos.length > 0 ? (
        <>
          <text fg={color.dim} attributes={DIM} wrapMode="none">
            ── recent changes ──
          </text>
          {diff.repos.map((r) => {
            const files = r.stat?.files ?? [];
            const added = files.reduce((a, f) => a + Math.max(f.added, 0), 0);
            const deleted = files.reduce((a, f) => a + Math.max(f.deleted, 0), 0);
            return (
              <text key={r.repo} wrapMode="none">
                <span fg={color.dim}>{glyph.filterMarker} </span>
                <span fg={color.repoHeader}>{r.repo}</span>
                <span fg={color.synced}> +{added}</span>
                <span fg={color.missing}> -{deleted}</span>
                <span fg={color.dim}> · {files.length} files</span>
              </text>
            );
          })}
        </>
      ) : null}
      {/* Session capture tail */}
      {capture && capture.lines.length > 0 ? (
        <>
          <text> </text>
          <text fg={color.dim} attributes={DIM} wrapMode="none">
            ── recent session output (live) ──
          </text>
          {capture.lines.map((l, i) => {
            const s = stripSgr(l);
            return (
              <text key={i} fg={color.dim} wrapMode="none">
                {s === "" ? " " : s}
              </text>
            );
          })}
        </>
      ) : null}
    </Panel>
  );
}

export function Browser(props: BrowserProps): ReactNode {
  const { views, enabled } = props;
  const [filterIndex, setFilterIndex] = useState(0);
  const [focusIndex, setFocusIndex] = useState(0);
  const [hubFocus, setHubFocus] = useState<"rail" | "list">("list");

  const filter = FILTERS[filterIndex]!;
  const visible = useMemo(() => {
    const list = views.filter(filter.match);
    if (filter.key === "8") {
      return [...list].sort((a, b) => {
        const r = attentionRank(a) - attentionRank(b);
        return r !== 0 ? r : a.slice.name.localeCompare(b.slice.name);
      });
    }
    return list;
  }, [views, filter]);

  // Keep focus in range as the visible set changes.
  useEffect(() => {
    setFocusIndex((i) => Math.max(0, Math.min(i, Math.max(0, visible.length - 1))));
  }, [visible.length]);

  const focusedSlice = visible[focusIndex];

  useKeyboard((key) => {
    if (!enabled) return;
    const name = key.name;
    if (name === "q") return props.onQuit();
    if (name === "?") return props.onToggleHelp();
    if (name === "P") return props.onToggleProcs();
    if (name === "r") return props.onRefresh();
    if (name === "tab") {
      setHubFocus((f) => (f === "rail" ? "list" : "rail"));
      return;
    }
    if (name >= "1" && name <= "8") {
      setFilterIndex(Number(name) - 1);
      setHubFocus("list");
      return;
    }
    if (name === "j" || name === "down") {
      if (hubFocus === "rail") setFilterIndex((i) => Math.min(FILTERS.length - 1, i + 1));
      else setFocusIndex((i) => Math.min(visible.length - 1, i + 1));
      return;
    }
    if (name === "k" || name === "up") {
      if (hubFocus === "rail") setFilterIndex((i) => Math.max(0, i - 1));
      else setFocusIndex((i) => Math.max(0, i - 1));
      return;
    }
    if (name === "g") return setFocusIndex(0);
    if (name === "G") return setFocusIndex(Math.max(0, visible.length - 1));
    if (name === "return" || name === "enter" || name === "l" || name === "right") {
      if (focusedSlice) props.onEnter(focusedSlice.slice.name);
      return;
    }
    if (name === "w") {
      if (focusedSlice) props.onSwap(focusedSlice.slice.name);
      return;
    }
    if (name === "a") {
      if (focusedSlice) props.onOpenTerm(focusedSlice.slice.name, false);
      return;
    }
    if (name === "C") {
      if (focusedSlice) props.onOpenTerm(focusedSlice.slice.name, true);
      return;
    }
  });

  const leftW = Math.max(20, Math.min(30, Math.floor(props.width / 4)));
  const bodyH = props.height - 2; // pulse bar + footer
  const railH = FILTERS.length + 2;
  const listH = bodyH - railH;

  return (
    <box flexDirection="column" width="100%" height="100%">
      <PulseBar count={views.length} views={views} version={props.version} />
      <box flexDirection="row" flexGrow={1}>
        <box flexDirection="column" width={leftW}>
          <StatesRail
            views={views}
            filterIndex={filterIndex}
            focused={hubFocus === "rail"}
            height={railH}
          />
          <Panel
            title={`Slices ${visible.length}`}
            focused={hubFocus === "list"}
            height={Math.max(3, listH)}
          >
            {visible.length === 0 ? (
              <text fg={color.dim} attributes={DIM}>
                (no slices in this filter)
              </text>
            ) : (
              visible.map((v, i) => (
                <SliceRow
                  key={v.slice.name}
                  view={v}
                  focused={i === focusIndex}
                  listFocused={hubFocus === "list"}
                />
              ))
            )}
          </Panel>
        </box>
        <Preview
          client={props.client}
          view={focusedSlice}
          conflicts={props.conflicts}
          height={bodyH}
        />
      </box>
      <text wrapMode="none" fg={color.dim} attributes={DIM}>
        tab rail/list · j/k move · 1-8 filter · enter open · a/C term · w live · P
        procs · r refresh · ? help · q quit
      </text>
    </box>
  );
}

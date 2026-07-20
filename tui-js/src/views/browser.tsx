import { useKeyboard } from "@opentui/react";
import type { KeyEvent } from "@opentui/core";
import { memo, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type {
  CaptureResult,
  ConflictsResult,
  DiffRepo,
  DiffResult,
  LsResult,
  PrStackEntry,
  RpcClient,
} from "../rpc/types";
import {
  attentionRank,
  FILTERS,
  isPhantom,
  needsRestack,
  needsYou,
  workState,
  type SliceView,
} from "../state/derive";
import {
  buildRows,
  clampFocus,
  clusterByStack,
  firstSelectable,
  focusIndexForSlice,
  isGatherableStackSlice,
  lastSelectable,
  missingSliceNames,
  searchAwareNav,
  stepSelectable,
  type BrowserRow,
  type StackLeader,
} from "../state/cluster";
import type { CockpitEntry } from "./cockpit.hints";
import { listHints } from "./browser.hints";
import { matchesSearch, toggleAllVisible, toggleSelected } from "../state/selection";
import { clampScroll, maxScroll } from "../util/scroll";
import { isQuitKey, normalizeKeyName } from "../util/keys";
import { shortcutAction } from "../util/shortcut-contract";
import { editText } from "../overlays/textinput";
import type { OverlayApi } from "../overlays/useOverlays";
import { attention, diffColor, glyph, theme } from "../theme";
import { StatStrip } from "../components/statstrip";
import { Eyebrow } from "../components/eyebrow";
import { Divider } from "../components/divider";
import { HintBar, type Hint } from "../components/hintbar";
import { StatusGlyph } from "../components/statusglyph";
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";
import type { OpenTermMode } from "../term/session";

export interface BrowserProps {
  enabled: boolean;
  client: RpcClient;
  version: string;
  workspaceRoot: string;
  views: SliceView[];
  ls: LsResult;
  conflicts: ConflictsResult | null;
  overlays: OverlayApi;
  width: number;
  height: number;
  onEnter: (slice: string, entry?: CockpitEntry) => void;
  onOpenTerm: (slice: string, mode: OpenTermMode) => void;
  onConfigureAgents: () => void;
  onFocusSlice: (slice: string) => void;
  initialFocusSlice?: string | null;
  focusRequest?: { id: number; slice: string } | null;
  onRefresh: () => void;
  onToggleProcs: () => void;
  onToggleSessions: () => void;
  onQuit: () => void;
  // Changes when the mutable semantic palette is switched; also invalidates
  // memoized slice rows so every cell repaints in the new theme.
  themeVersion: string;
  // Ambient background-create label for the header (spec D2), or null.
  createBusy?: string | null;
}

const PREVIEW_FILE_CAP = 8;

function filterColor(key: string): string {
  if (key === "4" || key === "7") return theme.good;
  if (key === "2" || key === "6" || key === "8") return theme.attn;
  return theme.textDim;
}

function conflictPartners(conflicts: ConflictsResult | null, slice: string): string[] {
  const set = new Set<string>();
  for (const o of conflicts?.overlaps ?? []) {
    if (!o.slices.includes(slice)) continue;
    for (const s of o.slices) if (s !== slice) set.add(s);
  }
  return [...set];
}

const SliceRow = memo(function SliceRow({
  view,
  focused,
  listFocused,
  selected,
  themeVersion: _themeVersion,
}: {
  view: SliceView;
  focused: boolean;
  listFocused: boolean;
  selected: boolean;
  themeVersion: string;
}): ReactNode {
  const a = attention(view);
  const focusRow = focused && listFocused;
  let nameColor: string = theme.text;
  let nameBold = false;
  if (a.level === 3) {
    nameColor = a.color;
    nameBold = true;
  } else if (a.level === 0) {
    nameColor = theme.textDim;
  }
  if (focusRow) {
    nameColor = theme.textBright;
    nameBold = true;
  }
  return (
    <box width="100%" backgroundColor={focusRow ? theme.surfaceAlt : undefined} flexDirection="row">
      <text wrapMode="none">
        <span fg={theme.focus} attributes={BOLD}>
          {focusRow ? glyph.focusBar : " "}
        </span>
        <span fg={selected ? theme.good : theme.textFaint}>
          {selected ? glyph.selected : " "}
        </span>
        <StatusGlyph view={view} />
        <span fg={nameColor} attributes={nameBold ? BOLD : 0}>
          {" "}
          {view.slice.name}
        </span>
        {view.slice.active ? <span fg={theme.good}> {glyph.live}</span> : null}
        {view.slice.stale ? <span fg={theme.attn}> {glyph.stale}</span> : null}
      </text>
    </box>
  );
});

function MissingRow({ name }: { name: string }): ReactNode {
  return (
    <box width="100%" flexDirection="row">
      <text wrapMode="none">
        <span fg={theme.textFaint}>{"   "}</span>
        <span fg={theme.textFaint} attributes={DIM}>
          {name}
        </span>
        <span fg={theme.bad}> missing</span>
      </text>
    </box>
  );
}

function StackHeader({ leader }: { leader: StackLeader }): ReactNode {
  const root = leader.root === "" ? "(stack)" : leader.root;
  return (
    <text wrapMode="none" fg={theme.textFaint} attributes={DIM}>
      {`  stack: ${root} → … (${leader.count} slices)`}
    </text>
  );
}

function FilterRail({
  views,
  filterIndex,
  focused,
  search,
  searching,
}: {
  views: SliceView[];
  filterIndex: number;
  focused: boolean;
  search: string;
  searching: boolean;
}): ReactNode {
  return (
    <box flexDirection="column">
      <Eyebrow label="Filters" focused={focused} />
      {FILTERS.map((f, i) => {
        const n = views.filter(f.match).length;
        const active = i === filterIndex;
        const countColor = filterColor(f.key);
        return (
          <text key={f.key} wrapMode="none">
            <span fg={active ? theme.focus : theme.textFaint}>
              {active ? `${glyph.filterMarker} ` : "  "}
            </span>
            <span fg={active ? theme.textBright : theme.textDim} attributes={active ? BOLD : 0}>
              {f.label.padEnd(13)}
            </span>
            <span fg={n > 0 ? countColor : theme.textFaint}>{String(n).padStart(2)}</span>
          </text>
        );
      })}
      {searching || search !== "" ? (
        <text wrapMode="none" fg={theme.focus}>
          {"  / "}
          <span fg={theme.text}>{search === "" ? "…" : search}</span>
        </text>
      ) : null}
    </box>
  );
}

function prBadge(prs: PrStackEntry[] | undefined, repo: string): ReactNode {
  const pr = prs?.find((p) => p.repo === repo);
  if (!pr || pr.number === undefined) {
    return <span fg={theme.textFaint}>{"  no PR"}</span>;
  }
  const state = (pr.state ?? "").toLowerCase();
  const stateColor =
    pr.state === "MERGED" ? theme.merged : pr.state === "OPEN" ? theme.good : theme.textDim;
  const ci =
    pr.ci === "pass"
      ? { g: glyph.ciPass, c: theme.good, t: "" }
      : pr.ci === "fail"
        ? { g: glyph.ciFail, c: theme.bad, t: (pr.ci_fail ?? 0) > 0 ? `${pr.ci_fail}` : "" }
        : pr.ci === "pending"
          ? { g: glyph.ciPending, c: theme.attn, t: "" }
          : null;
  const review =
    pr.review_decision === "APPROVED"
      ? { g: glyph.inReview, c: theme.good, t: "" }
      : pr.review_decision === "CHANGES_REQUESTED"
        ? { g: glyph.changes, c: theme.bad, t: " changes" }
        : null;
  return (
    <span>
      <span fg={theme.textFaint}>{`  #${pr.number} `}</span>
      <span fg={stateColor}>{state}</span>
      {ci ? (
        <span fg={ci.c}>
          {" "}
          {ci.g}
          {ci.t}
        </span>
      ) : null}
      {review ? (
        <span fg={review.c}>
          {" "}
          {review.g}
          {review.t}
        </span>
      ) : null}
    </span>
  );
}

function repoTotals(r: DiffRepo): { added: number; deleted: number; files: number } {
  const files = r.stat?.files ?? [];
  return {
    added: r.stat?.added ?? files.reduce((a, f) => a + Math.max(f.added, 0), 0),
    deleted: r.stat?.deleted ?? files.reduce((a, f) => a + Math.max(f.deleted, 0), 0),
    files: files.length,
  };
}

function previewLines(
  view: SliceView,
  diff: DiffResult | null,
  capture: CaptureResult | null,
  conflicts: ConflictsResult | null,
): ReactNode[] {
  const lines: ReactNode[] = [];
  const push = (n: ReactNode) => lines.push(n);
  const blank = () => push(<text key={`b${lines.length}`}> </text>);

  push(<Eyebrow key="e-state" label="State" bar={false} />);
  const overlaps = (conflicts?.overlaps ?? []).filter((o) => o.slices.includes(view.slice.name));
  const stateSpans: ReactNode[] = [];
  if (view.slice.active)
    stateSpans.push(
      <span key="live" fg={theme.good}>
        {glyph.live} live{"   "}
      </span>,
    );
  if (view.status === "waiting-input")
    stateSpans.push(
      <span key="wait" fg={theme.attn}>
        {glyph.waiting} needs you{"   "}
      </span>,
    );
  if (view.status === "running")
    stateSpans.push(
      <span key="running" fg={theme.good}>
        {glyph.running} agent running{"   "}
      </span>,
    );
  if (view.status === "done")
    stateSpans.push(
      <span key="done" fg={theme.merged}>
        {glyph.done} agent done{"   "}
      </span>,
    );
  if (workState(view) === "ready")
    stateSpans.push(
      <span key="ready" fg={theme.good}>
        {glyph.ready} ready to clear{"   "}
      </span>,
    );
  if (view.slice.stale)
    stateSpans.push(
      <span key="stale" fg={theme.attn}>
        {glyph.stale} primary behind tip{"   "}
      </span>,
    );
  if (overlaps.length > 0)
    stateSpans.push(
      <span key="ov" fg={theme.attn}>
        {glyph.overlap} overlaps {overlaps.length}
      </span>,
    );
  if (stateSpans.length === 0)
    stateSpans.push(
      <span key="idle" fg={theme.textDim}>
        idle
      </span>,
    );
  push(
    <text key="state" wrapMode="none">
      {"  "}
      {stateSpans}
    </text>,
  );
  if (isPhantom(view))
    push(
      <text key="phantom" wrapMode="none" fg={theme.bad}>
        {"  "}
        {glyph.dirty} doubled-prefix branch (phantom) — diff/PR won't match · run{" "}
        <span fg={theme.textBright}>slis doctor --fix</span>
      </text>,
    );
  blank();

  push(<Eyebrow key="e-repos" label="Repos" bar={false} />);
  for (const m of view.slice.members) {
    push(
      <text key={`repo-${m.repo}`} wrapMode="none">
        {"  "}
        <span fg={theme.focus} attributes={BOLD}>
          {m.repo}
        </span>
        <span fg={theme.text}>{"  " + m.branch}</span>
        {prBadge(view.prs, m.repo)}
      </text>,
    );
  }
  blank();

  push(<Eyebrow key="e-changes" label="Changes" trailing="vs working tree" bar={false} />);
  if (diff === null) {
    push(
      <text key="diff-loading" wrapMode="none" fg={theme.textDim} attributes={DIM}>
        {"  loading…"}
      </text>,
    );
  } else if (diff.repos.length === 0) {
    push(
      <text key="diff-none" wrapMode="none" fg={theme.textDim} attributes={DIM}>
        {"  no working-tree changes"}
      </text>,
    );
  } else {
    let shownFiles = 0;
    let totalFiles = 0;
    for (const r of diff.repos) {
      if (r.err) {
        push(
          <text key={`d-${r.repo}`} wrapMode="none">
            <span fg={theme.textFaint}>{glyph.filterMarker} </span>
            <span fg={theme.focus}>{r.repo}</span>
            <span fg={theme.bad}> diff unavailable</span>
          </text>,
        );
        continue;
      }
      const t = repoTotals(r);
      totalFiles += t.files;
      push(
        <text key={`d-${r.repo}`} wrapMode="none">
          <span fg={theme.textFaint}>{glyph.filterMarker} </span>
          <span fg={theme.focus}>{r.repo}</span>
          <span fg={diffColor.add}> +{t.added}</span>
          <span fg={diffColor.del}> -{t.deleted}</span>
          <span fg={theme.textDim}> · {t.files} files</span>
        </text>,
      );
      for (const file of r.stat?.files ?? []) {
        if (shownFiles >= PREVIEW_FILE_CAP) break;
        const binary = file.added < 0 || file.deleted < 0;
        push(
          <text key={`f-${r.repo}-${file.path}`} wrapMode="none">
            <span fg={theme.textFaint}>{"    "}</span>
            <span fg={theme.text}>{file.path}</span>
            {binary ? (
              <span fg={theme.textDim}> binary</span>
            ) : (
              <>
                <span fg={diffColor.add}> +{file.added}</span>
                <span fg={diffColor.del}> -{file.deleted}</span>
              </>
            )}
          </text>,
        );
        shownFiles++;
      }
    }
    if (totalFiles > shownFiles)
      push(
        <text key="files-more" wrapMode="none" fg={theme.textFaint} attributes={DIM}>
          {`    … ${totalFiles - shownFiles} more files · enter for stack details`}
        </text>,
      );
  }
  blank();

  push(<Eyebrow key="e-session" label="Session" trailing="live tail" bar={false} />);
  const capLines = (capture?.lines ?? []).map(stripSgr).filter((l) => l.trim() !== "");
  if (capLines.length === 0) {
    push(
      <text key="sess-none" wrapMode="none" fg={theme.textDim} attributes={DIM}>
        {"  no recent session output"}
      </text>,
    );
  } else {
    capLines.forEach((l, i) =>
      push(
        <text key={`sess-${i}`} wrapMode="none" fg={theme.textDim}>
          {"  "}
          <span fg={theme.textFaint}>{glyph.arrow} </span>
          {l}
        </text>,
      ),
    );
  }

  return lines;
}

function Preview({
  client,
  view,
  conflicts,
  height,
  scrollEnabled,
}: {
  client: RpcClient;
  view: SliceView | undefined;
  conflicts: ConflictsResult | null;
  height: number;
  scrollEnabled: boolean;
}): ReactNode {
  const [capture, setCapture] = useState<CaptureResult | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [scroll, setScroll] = useState(0);
  const slice = view?.slice.name;

  useEffect(() => {
    setScroll(0);
    if (!slice) return;
    let live = true;
    setCapture(null);
    setDiff(null);
    client.capture({ slice, lines: 3 }).then((r) => live && setCapture(r), () => {});
    client
      .diff({ slice, scope: "working", format: "stat" })
      .then((r) => live && setDiff(r), () => {});
    return () => {
      live = false;
    };
  }, [client, slice]);

  const lines = useMemo(
    () => (view ? previewLines(view, diff, capture, conflicts) : []),
    [view, diff, capture, conflicts],
  );

  const viewport = Math.max(3, height - 4);
  const start = clampScroll(scroll, lines.length, viewport);
  const halfPage = Math.max(1, Math.floor(viewport / 2));

  useKeyboard((key: KeyEvent) => {
    if (!scrollEnabled || !view) return;
    const name = key.name;
    if (key.ctrl && name === "d")
      setScroll((s) => clampScroll(s + halfPage, lines.length, viewport));
    else if (key.ctrl && name === "u")
      setScroll((s) => clampScroll(s - halfPage, lines.length, viewport));
    else if (name === "pagedown")
      setScroll((s) => clampScroll(s + viewport, lines.length, viewport));
    else if (name === "pageup")
      setScroll((s) => clampScroll(s - viewport, lines.length, viewport));
  });

  if (!view) {
    return (
      <box flexDirection="column" flexGrow={1} paddingLeft={2}>
        <text fg={theme.textDim} attributes={DIM}>
          Pick a slice to preview it.
        </text>
      </box>
    );
  }

  const shown = lines.slice(start, start + viewport);
  const overflow = maxScroll(lines.length, viewport);

  return (
    <box
      flexDirection="column"
      flexGrow={1}
      paddingLeft={2}
      overflow="hidden"
      onMouseScroll={(e) => {
        if (!scrollEnabled) return;
        const dir = e.scroll?.direction;
        if (dir === "down") setScroll((s) => clampScroll(s + 1, lines.length, viewport));
        else if (dir === "up") setScroll((s) => clampScroll(s - 1, lines.length, viewport));
      }}
    >
      <text wrapMode="none">
        <span fg={theme.textBright} attributes={BOLD}>
          {view.slice.name}
        </span>
        {overflow > 0 ? <span fg={theme.textFaint}>{`   ${start}/${overflow}`}</span> : null}
      </text>
      <Divider width={Math.max(10, height)} />
      {start > 0 ? (
        <text wrapMode="none" fg={theme.textFaint} attributes={DIM}>
          {`  ↑ ${start} more above`}
        </text>
      ) : null}
      {shown}
      {start + viewport < lines.length ? (
        <text wrapMode="none" fg={theme.textFaint} attributes={DIM}>
          {`  ↓ ${lines.length - start - viewport} more below`}
        </text>
      ) : null}
    </box>
  );
}

const RAIL_HINTS: Hint[] = [
  { key: "j/k", label: "filter" },
  { key: "tab", label: "to list" },
  { key: "1-8", label: "jump" },
  { key: "enter", label: "open" },
];

export function Browser(props: BrowserProps): ReactNode {
  const { views, enabled, overlays, onFocusSlice } = props;
  const [filterIndex, setFilterIndex] = useState(0);
  const [hubFocus, setHubFocus] = useState<"rail" | "list">("list");
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [searching, setSearching] = useState(false);
  const [search, setSearch] = useState("");

  const filter = FILTERS[filterIndex]!;
  const filtered = useMemo(() => {
    const list = views.filter((v) => filter.match(v) && matchesSearch(v.slice.name, search));
    if (filter.key === "8") {
      return [...list].sort((a, b) => {
        const r = attentionRank(a) - attentionRank(b);
        return r !== 0 ? r : a.slice.name.localeCompare(b.slice.name);
      });
    }
    return list;
  }, [views, filter, search]);

  const { ordered, leaders } = useMemo(() => {
    const c = clusterByStack(filtered);
    return { ordered: c.ordered, leaders: c.leaders };
  }, [filtered]);

  const missing = useMemo(() => missingSliceNames(props.ls.missing), [props.ls.missing]);
  const rows: BrowserRow[] = useMemo(() => buildRows(ordered, missing), [ordered, missing]);
  const [focusIndex, setFocusIndex] = useState(() =>
    focusIndexForSlice(rows, props.initialFocusSlice),
  );
  const appliedFocusRequest = useRef(0);

  useEffect(() => {
    const request = props.focusRequest;
    if (!request || appliedFocusRequest.current >= request.id) return;
    if (filterIndex !== 0) {
      setFilterIndex(0);
      return;
    }
    if (search !== "") {
      setSearch("");
      setSearching(false);
      return;
    }
    const requestedIndex = rows.findIndex(
      (row) => row.kind === "slice" && row.view.slice.name === request.slice,
    );
    if (requestedIndex < 0) return;
    setFocusIndex(requestedIndex);
    setHubFocus("list");
    appliedFocusRequest.current = request.id;
  }, [props.focusRequest, filterIndex, search, rows]);

  useEffect(() => {
    setFocusIndex((i) => clampFocus(rows, i));
  }, [rows]);

  const focusRow = rows[focusIndex];
  const focusedSlice = focusRow?.kind === "slice" ? focusRow.view : undefined;

  useEffect(() => {
    if (focusedSlice) onFocusSlice(focusedSlice.slice.name);
  }, [focusedSlice, onFocusSlice]);

  const targetsFor = (): string[] => {
    if (selected.size > 0) return [...selected];
    return focusedSlice ? [focusedSlice.slice.name] : [];
  };

  useKeyboard((key: KeyEvent) => {
    if (!enabled) return;
    const name = normalizeKeyName(key);
    const shortcut = shortcutAction("browser", name);

    if (searching) {
      if (name === "escape") {
        setSearch("");
        setSearching(false);
      } else if (name === "return" || name === "enter") {
        setSearching(false);
      } else {
        setSearch((s) => editText(s, key));
      }
      return;
    }

    if (isQuitKey(key, name)) return props.onQuit();
    if (name === "?") return overlays.help();
    if (name === "P") return props.onToggleProcs();
    if (name === "s") return props.onToggleSessions();
    if (name === "r") return props.onRefresh();
    if (name === "/") {
      setSearch("");
      setSearching(true);
      return;
    }
    if (name === "escape" && search !== "") {
      setSearch("");
      return;
    }
    if (name === "tab") {
      setHubFocus((f) => (f === "rail" ? "list" : "rail"));
      return;
    }
    if (name >= "1" && name <= "8") {
      setFilterIndex(Number(name) - 1);
      setHubFocus("list");
      return;
    }
    if (key.ctrl && (name === "d" || name === "u")) return;
    if (name === "pagedown" || name === "pageup") return;
    if (name === "j" || name === "down") {
      if (hubFocus === "rail") setFilterIndex((i) => Math.min(FILTERS.length - 1, i + 1));
      else setFocusIndex((i) => stepSelectable(rows, i, 1));
      return;
    }
    if (name === "k" || name === "up") {
      if (hubFocus === "rail") setFilterIndex((i) => Math.max(0, i - 1));
      else setFocusIndex((i) => stepSelectable(rows, i, -1));
      return;
    }
    if (name === "g") return setFocusIndex(firstSelectable(rows));
    if (name === "G") return setFocusIndex(lastSelectable(rows));
    if (name === "n") {
      const next = searchAwareNav(search !== "", rows, focusIndex, 1);
      if (next !== null) setFocusIndex(next);
      return;
    }
    if (name === "N") {
      const prev = searchAwareNav(search !== "", rows, focusIndex, -1);
      if (prev !== null) setFocusIndex(prev);
      return;
    }
    if (name === "return" || name === "enter" || name === "l" || name === "right") {
      if (focusedSlice) props.onEnter(focusedSlice.slice.name);
      return;
    }
    if (name === "space") {
      if (focusedSlice) setSelected((s) => toggleSelected(s, focusedSlice.slice.name));
      return;
    }
    if (name === "A") {
      setSelected((s) =>
        toggleAllVisible(
          s,
          rows.flatMap((r) => (r.kind === "slice" ? [r.view.slice.name] : [])),
        ),
      );
      return;
    }
    if (name === "w") {
      if (focusedSlice) overlays.swap(focusedSlice.slice.name, focusedSlice.slice.active);
      return;
    }
    if (name === "c") return overlays.create();
    if (name === "i") return overlays.candidates(props.ls.candidates ?? []);
    if (name === "I") return overlays.adopt();
    if (name === "m") {
      // Match the other batch actions: an explicit multi-selection wins, while
      // an empty selection acts on the focused row. A one-slice grouping is a
      // useful rename and, importantly, opens the real input instead of a
      // dead-end instructional modal.
      const targets = targetsFor();
      if (targets.length > 0) overlays.group(targets, () => setSelected(new Set()));
      return;
    }
    if (name === "u") {
      if (focusedSlice) overlays.ungroup(focusedSlice.slice.name);
      return;
    }
    if (name === "R") {
      const targets = targetsFor();
      if (targets.length > 0)
        overlays.stack(
          targets,
          conflictPartners(props.conflicts, targets[0]!),
          isGatherableStackSlice(views, targets[0]!),
        );
      return;
    }
    if (name === "!") return overlays.conflictRadar();
    if (name === "Y") {
      if (focusedSlice) overlays.yankPrStack(focusedSlice.slice.name);
      return;
    }
    if (shortcut === "clear-slice" && !key.ctrl) {
      const targets = targetsFor();
      if (targets.length === 0) return;
      const live = targets.filter((t) => views.find((v) => v.slice.name === t)?.slice.active);
      if (live.length > 0)
        overlays.info("Cannot clear", `${live.join(", ")} is live — swap back (w) first.`);
      else overlays.remove(targets);
      return;
    }
    if (shortcut === "configure-agents") return props.onConfigureAgents();
    // M4 — red CI is actionable from the browser. `v` opens the focused slice
    // straight into the cockpit PRs panel with the failing-CI log shown (one key
    // from red to why); `F` runs fix-ci through the existing PTY path.
    if (name === "v") {
      if (focusedSlice) props.onEnter(focusedSlice.slice.name, { panel: "prs", ciLog: true });
      return;
    }
    if (name === "F") {
      if (focusedSlice) overlays.fixCi(focusedSlice.slice.name);
      return;
    }
    if (shortcut === "attach-agent") {
      if (focusedSlice) props.onOpenTerm(focusedSlice.slice.name, "agent");
      return;
    }
    if (shortcut === "launch-agent") {
      if (focusedSlice) props.onOpenTerm(focusedSlice.slice.name, "agent-launch");
      return;
    }
    if (shortcut === "pending-review") {
      if (focusedSlice) overlays.review(focusedSlice.slice.name, props.onRefresh);
      return;
    }
    if (shortcut === "open-shell") {
      if (focusedSlice) props.onOpenTerm(focusedSlice.slice.name, "shell");
      return;
    }
    if (name === "e" || name === "o") {
      if (focusedSlice) overlays.editor(focusedSlice.slice.name);
      return;
    }
  });

  const leftW = Math.max(22, Math.min(32, Math.floor(props.width / 4)));
  const bodyH = props.height - 2;
  const sliceCount = rows.filter((r) => r.kind === "slice").length;
  const workspaceEmpty = views.length === 0 && missing.length === 0;

  const counts = {
    needsYou: views.filter(needsYou).length,
    live: views.filter((v) => v.slice.active).length,
    ready: views.filter((v) => workState(v) === "ready").length,
    restack: views.filter(needsRestack).length,
    errors:
      (props.ls.repo_errors?.length ?? 0) + (props.ls.skipped?.length ?? 0) + missing.length,
  };

  return (
    <box flexDirection="column" width="100%" height="100%">
      <StatStrip
        counts={counts}
        total={views.length}
        version={`v${props.version}`}
        busy={props.createBusy}
      />
      <box flexDirection="row" flexGrow={1}>
        <box
          flexDirection="column"
          width={leftW}
          border={["right"]}
          borderColor={theme.hairline}
          paddingRight={1}
          height={bodyH}
        >
          <FilterRail
            views={views}
            filterIndex={filterIndex}
            focused={hubFocus === "rail"}
            search={search}
            searching={searching}
          />
          <box
            marginTop={1}
            flexDirection="column"
            flexGrow={1}
            overflow="hidden"
            onMouseScroll={(e) => {
              if (!enabled) return;
              const dir = e.scroll?.direction;
              if (dir === "down") setFocusIndex((i) => stepSelectable(rows, i, 1));
              else if (dir === "up") setFocusIndex((i) => stepSelectable(rows, i, -1));
            }}
          >
            <Eyebrow label="Slices" focused={hubFocus === "list"} trailing={String(sliceCount)} />
            {workspaceEmpty ? (
              <>
                <text fg={theme.textDim}>No slices yet.</text>
                <text wrapMode="none" fg={theme.textDim} attributes={DIM}>
                  Press <span fg={theme.focus}>c</span> to create a feature slice,
                </text>
                <text wrapMode="none" fg={theme.textDim} attributes={DIM}>
                  or <span fg={theme.focus}>i</span> to import worktrees.
                </text>
              </>
            ) : sliceCount === 0 && missing.length === 0 ? (
              <text wrapMode="none" fg={theme.textDim} attributes={DIM}>
                {`Nothing matches "${filter.label}" — you're all caught up.`}
              </text>
            ) : (
              rows.map((row, i) => {
                if (row.kind === "missing")
                  return <MissingRow key={`m-${row.name}`} name={row.name} />;
                const leader = leaders.get(row.view.slice.name);
                return (
                  <box key={row.view.slice.name} flexDirection="column">
                    {leader ? <StackHeader leader={leader} /> : null}
                    <SliceRow
                      view={row.view}
                      focused={i === focusIndex}
                      listFocused={hubFocus === "list"}
                      selected={selected.has(row.view.slice.name)}
                      themeVersion={props.themeVersion}
                    />
                  </box>
                );
              })
            )}
          </box>
        </box>
        <Preview
          client={props.client}
          view={focusedSlice}
          conflicts={props.conflicts}
          height={bodyH}
          scrollEnabled={enabled && !searching}
        />
      </box>
      {searching ? (
        <text wrapMode="none" fg={theme.textDim} attributes={DIM}>
          type to filter · enter keep · esc clear
        </text>
      ) : (
        <HintBar
          hints={hubFocus === "rail" ? RAIL_HINTS : listHints(focusedSlice, search !== "")}
          width={props.width - 1}
        />
      )}
    </box>
  );
}

// Cockpit view: the lazygit-style detail screen for a single slice (spec §3.2).
//
//   slis › payroll-ssp-fix › Stack     ● LIVE · swapped in       esc back  ? help
//  ▎REPOS & STACK             2 repos  │ nory/Node-Middleware › Changes · working
//    …                                 │  …
//   ─────────────────────────────────  │
//    PRS                     2 open     │
//   ─────────────────────────────────  │
//    SESSION                ⏸ waiting   │
//   ─────────────────────────────────  │
//    PROCESSES                Σ 38%     │
//   ─────────────────────────────────────────────────────────────────────────────
//   tab panel   j/k repo   enter rich diff   b scope: working   w swap   ? more
//
// Left column: one seamless column of hairline-separated sections (focused
// section = bright eyebrow + ▎ bar + right-aligned headline summary). The right
// pane is the only bordered region — the content stage driven by the focus.

import { useKeyboard } from "@opentui/react";
import type { ScrollBoxRenderable } from "@opentui/core";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type {
  BranchDiffResult,
  CiLogResult,
  DiffResult,
  DiffScope,
  FileResult,
  PrComment,
  PrStackEntry,
  ProcsResult,
  ReviewComment,
  RpcClient,
} from "../rpc/types";
import { isMethodNotFound } from "../rpc/client";
import { fileComment, linesWithComments } from "../review/context";
import type { SliceView } from "../state/derive";
import { buildStackRows, clampSel } from "../state/stacknav";
import {
  flattenTree as flattenFileTree,
  parentPath,
  toggled,
  withChildren,
  type ChildrenByPath,
} from "../state/filetree";
import type { OverlayApi } from "../overlays/useOverlays";
import { commentBlocks } from "../pr/comments";
import { sessionName } from "../term/tmux";
import { color, glyph, sessionBadge, sessionLabel, theme } from "../theme";
import { Panel } from "../components/panel";
import { Breadcrumb } from "../components/breadcrumb";
import { Badge } from "../components/badge";
import { Divider } from "../components/divider";
import { HintBar } from "../components/hintbar";
import { DiffPane } from "../components/diffpane";
import { DiffView, type DiffMode } from "../components/diffview";
import { FileTree } from "../components/filetree";
import { FileView, contentLines } from "../components/fileview";
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";
import { normalizeKeyName } from "../util/keys";
import { useProcMonitor } from "../proc/monitor";
import { buildProcTree, flattenTree, totalCpu, totalMem } from "../proc/tree";
import { nextSort, SORT_LABEL, type ProcSort } from "../proc/sort";
import {
  applyKill,
  KillConfirm,
  KillStatusLine,
  ProcTableHeader,
  ProcTotalsRow,
  ProcTreeRow,
  SLICE_CPU_WARN,
  type KillStatus,
  type KillTarget,
} from "../components/procview";
import {
  breadcrumbSection,
  cockpitHints,
  cyclePanel,
  PANEL_ORDER,
  type PanelId,
  type ReviewMode,
} from "./cockpit.hints";

const SCOPES: DiffScope[] = ["working", "parent", "trunk"];
const SCOPE_SHORT: Record<DiffScope, string> = {
  working: "working",
  parent: "parent",
  trunk: "trunk",
};

// A short session-state word for the eyebrow's right-aligned headline.
function sessionSummary(status: SliceView["status"]): string {
  switch (status) {
    case "waiting-input":
      return "waiting";
    case "running":
      return "running";
    case "done":
      return "done";
    default:
      return "idle";
  }
}

// ciBadge maps a PR's CI rollup to a coloured glyph (failing rollups append the
// failing-check count, e.g. ✗2). Returns null when the PR carries no CI data.
function ciBadge(pr: PrStackEntry): { glyph: string; color: string } | null {
  if (!pr.ci) return null;
  if (pr.ci === "pass") return { glyph: glyph.ciPass, color: color.synced };
  if (pr.ci === "fail")
    return { glyph: glyph.ciFail + (pr.ci_fail ? String(pr.ci_fail) : ""), color: color.missing };
  return { glyph: glyph.ciPending, color: color.wait };
}

export interface CockpitProps {
  enabled: boolean;
  client: RpcClient;
  view: SliceView;
  overlays: OverlayApi;
  width: number;
  height: number;
  // Entry focus when opened from the browser (M4): which panel to land on and
  // whether to auto-open the focused PR's failing-CI log.
  initialPanel?: PanelId;
  openCiLog?: boolean;
  onBack: () => void;
  onOpenTerm: (slice: string, launchAgent: boolean) => void;
  onToggleProcs: () => void;
  onRefresh: () => void;
  onQuit: () => void;
}

function useCapture(
  client: RpcClient,
  slice: string,
  tick: boolean,
  reloadNonce: number,
): string[] {
  const [lines, setLines] = useState<string[]>([]);
  useEffect(() => {
    let live = true;
    const load = () =>
      client
        .capture({ slice, lines: 200 })
        .then((r) => live && setLines(r.lines.map(stripSgr)), () => {});
    load();
    if (!tick) return () => {
      live = false;
    };
    const id = setInterval(load, 2000);
    return () => {
      live = false;
      clearInterval(id);
    };
  }, [client, slice, tick, reloadNonce]);
  return lines;
}

// RepoComments-per-repo for a slice, lazily loaded when the PRs panel is focused.
type SliceComments = Record<string, { pr: number; url: string; comments: PrComment[] }>;

function useComments(
  client: RpcClient,
  slice: string,
  active: boolean,
): SliceComments {
  const [byRepo, setByRepo] = useState<SliceComments>({});
  useEffect(() => {
    setByRepo({});
    if (!active) return;
    let live = true;
    client
      .comments(slice)
      .then((res) => live && setByRepo(res[slice] ?? {}), () => {});
    return () => {
      live = false;
    };
  }, [client, slice, active]);
  return byRepo;
}

// ── left sections (seamless — eyebrow + hairline, no box) ─────────────────────

function StackSection({
  view,
  focused,
  rows,
  sel,
  flexGrow,
}: {
  view: SliceView;
  focused: boolean;
  rows: ReturnType<typeof buildStackRows>;
  sel: number;
  flexGrow?: number;
}): ReactNode {
  const n = view.slice.members.length;
  return (
    <Panel
      title="Repos & Stack"
      variant="seamless"
      focused={focused}
      flexGrow={flexGrow}
      trailing={`${n} ${n === 1 ? "repo" : "repos"}`}
    >
      {rows.map((row, i) => {
        const selected = i === sel && focused;
        const c = row.trunk
          ? color.synced
          : row.needsRestack
            ? color.restack
            : row.isMember
              ? color.white
              : color.fg;
        return (
          <box key={`${row.repo}\t${row.branch}`} flexDirection="column">
            {row.firstOfRepo ? (
              <text wrapMode="none">
                <span fg={color.cursorBar}> </span>
                <span fg={color.repoHeader} attributes={BOLD}>
                  {row.repo}
                </span>
              </text>
            ) : null}
            <text wrapMode="none">
              <span fg={color.cursorBar}>{selected ? glyph.focusBar : " "}</span>
              <span fg={color.dim}>{"  " + "  ".repeat(Math.max(0, row.depth))}</span>
              <span fg={c} attributes={row.isMember || selected ? BOLD : 0}>
                {row.branch}
              </span>
              {row.trunk ? <span fg={color.synced}> [trunk]</span> : null}
              {row.needsRestack ? (
                <span fg={color.restack}> {glyph.restack} restack</span>
              ) : null}
            </text>
          </box>
        );
      })}
    </Panel>
  );
}

function PrsSection({
  view,
  focused,
  prSel,
}: {
  view: SliceView;
  focused: boolean;
  prSel: number;
}): ReactNode {
  const prs = view.prs ?? [];
  const loading = view.prs === undefined;
  const openCount = prs.filter((p) => p.state === "OPEN").length;
  return (
    <Panel
      title="PRs"
      variant="seamless"
      focused={focused}
      trailing={loading ? "…" : `${openCount} open`}
    >
      {prs.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          {loading ? "loading…" : "no branches"}
        </text>
      ) : (
        prs.map((pr, i) => {
          const selected = i === prSel;
          const stateColor =
            pr.state === "MERGED"
              ? color.merged
              : pr.state === "OPEN"
                ? color.synced
                : color.dim;
          const ci = ciBadge(pr);
          return (
            <text key={pr.repo + pr.branch} wrapMode="none">
              <span fg={color.cursorBar}>
                {selected && focused ? glyph.focusBar : " "}
              </span>
              <span fg={color.repoHeader}>{pr.repo}</span>
              {pr.number !== undefined ? (
                <>
                  <span fg={color.dim}> #{pr.number} </span>
                  <span fg={stateColor}>{(pr.state ?? "").toLowerCase()}</span>
                  {ci ? <span fg={ci.color}> {ci.glyph}</span> : null}
                  {pr.review_decision === "APPROVED" ? (
                    <span fg={color.synced}> {glyph.inReview}</span>
                  ) : pr.review_decision === "CHANGES_REQUESTED" ? (
                    <span fg={color.missing}> {glyph.changes}</span>
                  ) : null}
                </>
              ) : (
                <span fg={color.dim}> (no PR)</span>
              )}
            </text>
          );
        })
      )}
    </Panel>
  );
}

function SessionSection({
  view,
  focused,
  lastLine,
}: {
  view: SliceView;
  focused: boolean;
  lastLine: string | undefined;
}): ReactNode {
  const badge = sessionBadge(view.status);
  const trailing = (
    <text wrapMode="none">
      <span fg={badge.color}>
        {badge.glyph} {sessionSummary(view.status)}
      </span>
    </text>
  );
  return (
    <Panel title="Session" variant="seamless" focused={focused} trailing={trailing}>
      {lastLine ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          {"  " + glyph.arrow + " " + lastLine}
        </text>
      ) : (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          {"  " + sessionLabel(view.status)}
        </text>
      )}
    </Panel>
  );
}

function ProcsSection({
  procs,
  focused,
}: {
  procs: ProcsResult | null;
  focused: boolean;
}): ReactNode {
  const slice = procs?.slices[0];
  const total = slice?.totalCPU ?? 0;
  const over = total > SLICE_CPU_WARN;
  const trailing = (
    <text wrapMode="none">
      <span fg={over ? color.restack : color.dim} attributes={over ? BOLD : 0}>
        Σ {total.toFixed(0)}%
      </span>
    </text>
  );
  return (
    <Panel title="Processes" variant="seamless" focused={focused} trailing={trailing}>
      {!procs ? (
        <text fg={color.dim} attributes={DIM}>
          sampling…
        </text>
      ) : !slice || slice.procs.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          no session / no processes
        </text>
      ) : (
        slice.procs.slice(0, 2).map((p) => (
          <text key={p.pid} wrapMode="none">
            <span fg={color.live}>{"  " + glyph.live + " "}</span>
            <span fg={color.fg}>{p.cmd}</span>
            <span fg={color.dim}> {p.cpu.toFixed(0)}%</span>
          </text>
        ))
      )}
    </Panel>
  );
}

// ── right pane ───────────────────────────────────────────────────────────────

function DiffRight({
  diff,
  repo,
  scope,
  showPatch,
}: {
  diff: DiffResult | null;
  repo: string;
  scope: DiffScope;
  showPatch: boolean;
}): ReactNode {
  if (!diff) {
    return (
      <text fg={color.dim} attributes={DIM}>
        loading diff…
      </text>
    );
  }
  const rd = diff.repos.find((r) => r.repo === repo);
  return (
    <DiffPane
      repo={repo}
      stat={rd?.stat}
      patch={rd?.patch}
      err={rd?.err}
      scope={scope}
      showPatch={showPatch}
    />
  );
}

// BranchDiffRight renders a non-member branch's diff-vs-parent (F3). It reuses
// DiffPane with the "parent" scope label; the panel title carries the parent ref.
function BranchDiffRight({
  bd,
  repo,
  showPatch,
}: {
  bd: BranchDiffResult | null;
  repo: string;
  showPatch: boolean;
}): ReactNode {
  if (!bd) {
    return (
      <text fg={color.dim} attributes={DIM}>
        loading diff…
      </text>
    );
  }
  return (
    <DiffPane
      repo={repo}
      stat={bd.stat}
      patch={bd.patch}
      err={bd.err}
      scope="parent"
      showPatch={showPatch}
    />
  );
}

function CommentsBlock({
  repo,
  prNumber,
  comments,
  width,
}: {
  repo: string;
  prNumber: number;
  comments: PrComment[];
  width: number;
}): ReactNode {
  if (comments.length === 0) return null;
  const blocks = commentBlocks(repo, prNumber, comments, Math.max(20, width - 4));
  return (
    <>
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        Comments:
      </text>
      {blocks.map((b, i) => (
        <box key={i} flexDirection="column">
          <text fg={color.title} attributes={BOLD} wrapMode="none">
            {b.header}
          </text>
          {b.body.map((l, j) => (
            <text key={j} fg={color.fg} wrapMode="none">
              {l === "" ? " " : l}
            </text>
          ))}
        </box>
      ))}
    </>
  );
}

function PrDetailRight({
  view,
  prSel,
  comments,
  width,
}: {
  view: SliceView;
  prSel: number;
  comments: SliceComments;
  width: number;
}): ReactNode {
  const pr = (view.prs ?? [])[prSel];
  if (!pr) return <text fg={color.dim}>no PR selected</text>;
  const rc = comments[pr.repo];
  if (pr.number === undefined) {
    // No live PR — fall back to any cached comments for this repo.
    if (rc && rc.comments.length > 0) {
      return (
        <>
          <text fg={color.dim} attributes={DIM} wrapMode="none">
            {pr.repo} · {pr.branch} — no open PR · cached comments
          </text>
          <CommentsBlock repo={pr.repo} prNumber={rc.pr} comments={rc.comments} width={width} />
        </>
      );
    }
    return (
      <text fg={color.dim} attributes={DIM}>
        {pr.repo} · {pr.branch} — no PR opened
      </text>
    );
  }
  return (
    <>
      <text wrapMode="none">
        <span fg={color.repoHeader} attributes={BOLD}>
          {pr.repo}
        </span>
        <span fg={color.dim}> #{pr.number} · </span>
        <span fg={pr.state === "MERGED" ? color.merged : color.synced}>
          {pr.state}
        </span>
      </text>
      <text fg={color.white} wrapMode="none">
        {pr.title ?? ""}
      </text>
      <text> </text>
      <text wrapMode="none">
        <span fg={color.dim}>review: </span>
        <span
          fg={
            pr.review_decision === "APPROVED"
              ? color.synced
              : pr.review_decision === "CHANGES_REQUESTED"
                ? color.missing
                : color.dim
          }
        >
          {pr.review_decision || "—"}
        </span>
      </text>
      <text fg={color.dim} wrapMode="none">
        {pr.url ?? ""}
      </text>
      {rc ? (
        <CommentsBlock repo={pr.repo} prNumber={rc.pr} comments={rc.comments} width={width} />
      ) : null}
    </>
  );
}

interface CiLogState {
  repo: string;
  loading: boolean;
  result: CiLogResult | null;
}

function CiLogRight({ state }: { state: CiLogState }): ReactNode {
  if (state.loading) {
    return (
      <text fg={color.dim} attributes={DIM}>
        loading CI log…
      </text>
    );
  }
  const repo = state.result?.repos.find((r) => r.repo === state.repo);
  if (!repo || (!repo.log && !repo.error)) {
    return (
      <text fg={color.dim} attributes={DIM}>
        no CI log for {state.repo}
      </text>
    );
  }
  if (repo.error) {
    return (
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        {repo.error}
      </text>
    );
  }
  const lines = (repo.log ?? "").split("\n");
  return (
    <>
      {lines.map((l, i) => (
        <text key={i} fg={color.fg} wrapMode="none">
          {l === "" ? " " : stripSgr(l)}
        </text>
      ))}
    </>
  );
}

function SessionRight({
  view,
  lines,
}: {
  view: SliceView;
  lines: string[];
}): ReactNode {
  const members = view.slice.members;
  return (
    <>
      <text wrapMode="none">
        <span fg={color.dim}>session: </span>
        <span fg={color.fg}>{sessionName(view.slice.name)}</span>
        <span fg={color.dim}> · {members.length} windows</span>
      </text>
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        repos:
      </text>
      {members.map((m) => (
        <text key={m.repo} wrapMode="none">
          <span fg={color.repoHeader}>{"  " + m.repo}</span>
          <span fg={color.dim}>{"  " + m.worktree_path}</span>
        </text>
      ))}
      <text> </text>
      {lines.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          (no session output — attach with a/C to start one)
        </text>
      ) : (
        <>
          <text fg={color.dim} attributes={DIM} wrapMode="none">
            ─── session output (live) ───
          </text>
          {lines.map((l, i) => (
            <text key={i} fg={color.fg} wrapMode="none">
              {l === "" ? " " : l}
            </text>
          ))}
        </>
      )}
    </>
  );
}

function ProcsRight({
  monitor,
  rows,
  procSel,
  sort,
  pendingKill,
  killStatus,
}: {
  monitor: ReturnType<typeof useProcMonitor>;
  rows: ReturnType<typeof flattenTree>;
  procSel: number;
  sort: ProcSort;
  pendingKill: KillTarget | null;
  killStatus: KillStatus | null;
}): ReactNode {
  const { result, history } = monitor;
  const slice = result?.slices[0];
  if (!result) return <text fg={color.dim} attributes={DIM}>sampling…</text>;
  if (!slice || slice.procs.length === 0)
    return (
      <text fg={color.dim} attributes={DIM}>
        no tmux session / no processes
      </text>
    );
  return (
    <>
      <text wrapMode="none">
        <span fg={color.dim} attributes={DIM}>
          {`sort: ${SORT_LABEL[sort]}  ·  s cycle · l/→ expand · h/← collapse · x kill · X kill tree`}
        </span>
      </text>
      <ProcTableHeader spark />
      {rows.map((row, i) => (
        <ProcTreeRow
          key={row.proc.pid}
          row={row}
          selected={i === procSel}
          history={history}
          spark
        />
      ))}
      <ProcTotalsRow
        cpu={totalCpu(slice.procs)}
        mem={totalMem(slice.procs)}
        count={slice.procs.length}
      />
      {pendingKill ? (
        <>
          <text> </text>
          <KillConfirm target={pendingKill} />
        </>
      ) : killStatus ? (
        <>
          <text> </text>
          <KillStatusLine status={killStatus} />
        </>
      ) : null}
    </>
  );
}

// ── cockpit ──────────────────────────────────────────────────────────────────

export function Cockpit(props: CockpitProps): ReactNode {
  const { view, client, enabled, overlays } = props;
  const slice = view.slice.name;

  const [panel, setPanel] = useState<PanelId>(props.initialPanel ?? "stack");
  // Stack panel selection is tracked by branch identity (repo\tbranch), not a
  // raw index, so it survives the stack rows growing when `show` (the lineage)
  // arrives after the first paint. An empty key means "default to the member".
  const [stackSelKey, setStackSelKey] = useState("");
  const [prSel, setPrSel] = useState(0);
  const [procSel, setProcSel] = useState(0);
  const [procSort, setProcSort] = useState<ProcSort>("cpu");
  const [collapsed, setCollapsed] = useState<Set<number>>(() => new Set());
  const [pendingKill, setPendingKill] = useState<KillTarget | null>(null);
  const [killStatus, setKillStatus] = useState<KillStatus | null>(null);
  const [scopeIdx, setScopeIdx] = useState(0);
  const [showPatch, setShowPatch] = useState(false);
  const [ciLog, setCiLog] = useState<CiLogState | null>(null);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [diffOpen, setDiffOpen] = useState(false);
  const [diffMode, setDiffMode] = useState<DiffMode>("unified");
  const [zoomed, setZoomed] = useState(false);
  const [captureNonce, setCaptureNonce] = useState(0);
  // F3 stack-review state.
  const [reviewMode, setReviewMode] = useState<ReviewMode>("diff");
  const [branchDiff, setBranchDiff] = useState<BranchDiffResult | null>(null);
  const [treeChildren, setTreeChildren] = useState<ChildrenByPath>({});
  const [treeExpanded, setTreeExpanded] = useState<Set<string>>(() => new Set());
  const [treeSel, setTreeSel] = useState(0);
  const [treeLoading, setTreeLoading] = useState(false);
  const [openFile, setOpenFile] = useState<FileResult | null>(null);
  const [fileLoading, setFileLoading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);
  const [stackReviewSupported, setStackReviewSupported] = useState(true);
  // F2 inline review: pending comments for this slice + a line cursor for the
  // file view. `reviewsNonce` piggybacks reloads on other actions (add/send/
  // refresh) — no dedicated tick.
  const [reviews, setReviews] = useState<ReviewComment[]>([]);
  const [reviewsNonce, setReviewsNonce] = useState(0);
  const [reviewsSupported, setReviewsSupported] = useState(true);
  const [fileCursor, setFileCursor] = useState(0);
  const scrollRef = useRef<ScrollBoxRenderable>(null);

  const scope = SCOPES[scopeIdx]!;

  const stackRows = useMemo(() => buildStackRows(view), [view]);
  const defaultStackSel = useMemo(() => {
    const m = stackRows.findIndex((r) => r.isMember);
    return m >= 0 ? m : 0;
  }, [stackRows]);
  const stackSel = useMemo(() => {
    if (stackSelKey === "") return defaultStackSel;
    const i = stackRows.findIndex((r) => `${r.repo}\t${r.branch}` === stackSelKey);
    return i >= 0 ? i : defaultStackSel;
  }, [stackRows, stackSelKey, defaultStackSel]);
  const stackRow = stackRows[stackSel];
  const selectedRepo = stackRow?.repo ?? view.slice.members[0]?.repo ?? "";
  const selectedBranch = stackRow?.branch ?? "";
  const onMemberBranch = stackRow?.isMember ?? true;

  const treeRows = useMemo(
    () => flattenFileTree(treeChildren, treeExpanded),
    [treeChildren, treeExpanded],
  );

  // Lines of the open file (for the review line cursor) + which of them carry a
  // pending comment (for the ✎ gutter marker).
  const fileLines = useMemo(
    () => (openFile && !openFile.binary ? contentLines(openFile.content ?? "") : []),
    [openFile],
  );
  const fileMarked = useMemo(
    () => (openFile ? linesWithComments(reviews, selectedRepo, openFile.path) : new Set<number>()),
    [reviews, selectedRepo, openFile],
  );

  const monitor = useProcMonitor(client, slice, panel === "procs");
  const comments = useComments(client, slice, panel === "prs");
  const sliceProcs = monitor.result?.slices[0]?.procs ?? [];
  const procRows = useMemo(
    () => flattenTree(buildProcTree(sliceProcs, procSort), collapsed),
    [sliceProcs, procSort, collapsed],
  );
  const captureLines = useCapture(client, slice, panel === "session", captureNonce);
  const lastLine = captureLines[captureLines.length - 1];

  // Load the slice diff for the stack panel's right pane (member branch, scoped
  // working/parent/trunk); refetch on slice/scope change.
  useEffect(() => {
    if (panel !== "stack") return;
    let live = true;
    setDiff(null);
    client
      .diff({ slice, scope, format: "both" })
      .then((r) => live && setDiff(r), () => {});
    return () => {
      live = false;
    };
  }, [client, slice, scope, panel]);

  // Load the branch-vs-parent diff when a NON-member branch is selected (F3). A
  // method-not-found means the sidecar predates F3 — hide the feature and fall
  // back to the member-scoped diff for every node.
  useEffect(() => {
    if (panel !== "stack" || reviewMode !== "diff" || onMemberBranch) return;
    if (!stackReviewSupported || !selectedBranch) return;
    let live = true;
    setBranchDiff(null);
    client.branchDiff({ slice, repo: selectedRepo, branch: selectedBranch, format: "both" }).then(
      (r) => live && setBranchDiff(r),
      (err) => {
        if (isMethodNotFound(err)) setStackReviewSupported(false);
      },
    );
    return () => {
      live = false;
    };
  }, [
    client,
    slice,
    panel,
    reviewMode,
    onMemberBranch,
    selectedRepo,
    selectedBranch,
    stackReviewSupported,
  ]);

  // Load the slice's pending review comments (F2). Reloads on slice change and
  // whenever `reviewsNonce` bumps (after add / send / refresh) — no new tick. An
  // older sidecar without the `reviews` method disables the feature silently.
  useEffect(() => {
    if (!reviewsSupported) return;
    let live = true;
    client.reviews({ slice }).then(
      (rows) => live && setReviews(rows),
      (err) => {
        if (isMethodNotFound(err)) setReviewsSupported(false);
      },
    );
    return () => {
      live = false;
    };
  }, [client, slice, reviewsNonce, reviewsSupported]);

  const bumpReviews = () => setReviewsNonce((n) => n + 1);

  // Leaving the Stack panel (or changing slice) drops back to the diff sub-mode.
  useEffect(() => {
    setReviewMode("diff");
  }, [panel, slice]);

  // A fresh slice starts from the default (member) branch selection.
  useEffect(() => {
    setStackSelKey("");
  }, [slice]);

  // Reset scroll to top whenever the right-pane content identity changes.
  useEffect(() => {
    scrollRef.current?.scrollTo(0);
  }, [
    panel,
    selectedRepo,
    selectedBranch,
    scope,
    showPatch,
    reviewMode,
    openFile?.path,
    prSel,
    ciLog?.repo,
    ciLog?.loading,
  ]);

  // Drop any open CI log when the focus moves off it (new PR, panel, or slice).
  useEffect(() => {
    setCiLog(null);
  }, [slice, panel, prSel]);

  const moveSel = (delta: number) => {
    if (panel === "stack") {
      const next = clampSel(stackSel + delta, stackRows.length);
      const row = stackRows[next];
      if (row) setStackSelKey(`${row.repo}\t${row.branch}`);
    } else if (panel === "prs")
      setPrSel((i) => Math.max(0, Math.min((view.prs?.length ?? 1) - 1, i + delta)));
    else if (panel === "procs")
      setProcSel((i) => Math.max(0, Math.min(procRows.length - 1, i + delta)));
  };

  // ── F3 file-tree browser ────────────────────────────────────────────────────

  // fetchLevel loads one directory level of the selected branch's tree and stores
  // it. A method-not-found means the sidecar predates F3 — hide the feature.
  const fetchLevel = (path: string) => {
    setTreeLoading(true);
    client.tree({ slice, repo: selectedRepo, branch: selectedBranch, path }).then(
      (r) => {
        setTreeChildren((c) => withChildren(c, path, r.entries));
        setTreeLoading(false);
      },
      (err) => {
        if (isMethodNotFound(err)) setStackReviewSupported(false);
        setTreeLoading(false);
      },
    );
  };

  const openTree = () => {
    if (!stackReviewSupported || !selectedBranch) return;
    setTreeChildren({});
    setTreeExpanded(new Set());
    setTreeSel(0);
    setReviewMode("tree");
    fetchLevel("");
  };

  const openFileAt = (path: string) => {
    setOpenFile(null);
    setFileError(null);
    setFileLoading(true);
    setFileCursor(0);
    setReviewMode("file");
    client.file({ slice, repo: selectedRepo, branch: selectedBranch, path }).then(
      (r) => {
        setOpenFile(r);
        setFileLoading(false);
      },
      (err) => {
        if (isMethodNotFound(err)) setStackReviewSupported(false);
        setFileError(err instanceof Error ? err.message : String(err));
        setFileLoading(false);
      },
    );
  };

  // l/enter in the tree: expand/collapse a directory (fetching its children the
  // first time), or open a file.
  const treeActivate = () => {
    const row = treeRows[treeSel];
    if (!row) return;
    if (row.type === "tree") {
      const willExpand = !treeExpanded.has(row.path);
      setTreeExpanded((e) => toggled(e, row.path));
      if (willExpand && !treeChildren[row.path]) fetchLevel(row.path);
    } else if (row.type === "blob") {
      openFileAt(row.path);
    }
  };

  // h/← in the tree: collapse the selected directory, else collapse+select its
  // parent directory.
  const treeCollapse = () => {
    const row = treeRows[treeSel];
    if (!row) return;
    if (row.type === "tree" && row.expanded) {
      setTreeExpanded((e) => toggled(e, row.path));
      return;
    }
    const parent = parentPath(row.path);
    if (parent === "") return;
    const pi = treeRows.findIndex((r) => r.path === parent);
    if (pi >= 0) {
      setTreeSel(pi);
      setTreeExpanded((e) => toggled(e, parent));
    }
  };

  // File-view line cursor (F2): move + keep the cursor line in view.
  const moveFileCursor = (delta: number) => {
    setFileCursor((i) => {
      const next = Math.max(0, Math.min(Math.max(0, fileLines.length - 1), i + delta));
      scrollRef.current?.scrollChildIntoView(`fileline-${next}`);
      return next;
    });
  };

  // Comment on the file view's cursor line — captures the line + surrounding
  // source as the excerpt, then opens the composer.
  const commentOnFileLine = () => {
    if (!openFile || fileLines.length === 0) return;
    const fc = fileComment(fileLines, fileCursor);
    overlays.comment(
      {
        slice,
        repo: selectedRepo,
        branch: selectedBranch,
        file: openFile.path,
        line: fc.line,
        hunk: fc.hunk,
      },
      bumpReviews,
    );
  };

  const openReviewOverlay = () => overlays.review(slice, bumpReviews);

  const toggleCollapse = (expand: boolean) => {
    const pid = procRows[procSel]?.proc.pid;
    if (pid === undefined) return;
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (expand) next.delete(pid);
      else next.add(pid);
      return next;
    });
  };

  const requestKill = (subtree: boolean) => {
    const row = procRows[procSel];
    if (!row) return;
    setKillStatus(null);
    setPendingKill({ pid: row.proc.pid, subtree, cmd: row.proc.cmd });
  };

  const confirmKill = () => {
    if (!pendingKill) return;
    setKillStatus(applyKill(sliceProcs, pendingKill));
    setPendingKill(null);
  };

  const focusedPr = () =>
    panel === "prs"
      ? (view.prs ?? [])[prSel]
      : (view.prs ?? []).find((p) => p.repo === selectedRepo);

  // Toggle the failing-CI log for the focused PR into the right pane. Fetched
  // lazily via the read-only sidecar; a second press closes it.
  const toggleCiLog = () => {
    const pr = focusedPr();
    if (!pr || pr.number === undefined) return;
    if (ciLog && ciLog.repo === pr.repo) {
      setCiLog(null);
      return;
    }
    const repo = pr.repo;
    setCiLog({ repo, loading: true, result: null });
    client.ciLog({ slice, repo }).then(
      (r) => setCiLog((c) => (c && c.repo === repo ? { repo, loading: false, result: r } : c)),
      () =>
        setCiLog((c) =>
          c && c.repo === repo ? { repo, loading: false, result: { repos: [] } } : c,
        ),
    );
  };

  // M4 — when entered from a red-CI browser slice (openCiLog), land on the first
  // failing PR and open its CI log without a keystroke. Waits for view.prs to
  // load, then runs exactly once.
  const autoCiOpenedRef = useRef(false);
  useEffect(() => {
    if (!props.openCiLog || autoCiOpenedRef.current) return;
    const prs = view.prs ?? [];
    const failing = prs.findIndex(
      (p) => p.number !== undefined && (p.ci === "fail" || (p.ci_fail ?? 0) > 0),
    );
    const target = failing >= 0 ? failing : prs.findIndex((p) => p.number !== undefined);
    if (target < 0) return; // PRs not loaded yet — retry when view.prs arrives
    autoCiOpenedRef.current = true;
    setPrSel(target);
    const repo = prs[target]!.repo;
    setCiLog({ repo, loading: true, result: null });
    client.ciLog({ slice, repo }).then(
      (r) => setCiLog((c) => (c && c.repo === repo ? { repo, loading: false, result: r } : c)),
      () =>
        setCiLog((c) =>
          c && c.repo === repo ? { repo, loading: false, result: { repos: [] } } : c,
        ),
    );
  }, [props.openCiLog, view.prs, client, slice]);

  const yankDiff = () => {
    const build = diff
      ? Promise.resolve(diff)
      : client.diff({ slice, scope, format: "patch" });
    build.then(
      (d) =>
        overlays.yankDiff(
          d.repos.map((r) => `# repo: ${r.repo}\n${r.patch ?? ""}`).join("\n"),
        ),
      () => {},
    );
  };

  // Half a right-pane page (D7 space scroll / parity with Go HalfPageDown).
  const halfPage = Math.max(1, Math.floor((props.height - 2) / 2));

  // Refresh inside the cockpit (G10): on the Session panel reload only the live
  // capture; otherwise trigger the app-level workspace refresh.
  const refreshCockpit = () => {
    bumpReviews();
    if (panel === "session") setCaptureNonce((n) => n + 1);
    else props.onRefresh();
  };

  useKeyboard((key) => {
    if (!enabled) return;
    // While the full diff view is open it owns the keyboard.
    if (diffOpen) return;
    const name = normalizeKeyName(key);

    // A pending kill confirmation captures input until answered.
    if (pendingKill) {
      if (name === "y" || name === "return" || name === "enter") return confirmKill();
      if (name === "n" || name === "escape") return setPendingKill(null);
      return;
    }

    // F3 stack-review sub-modes own navigation + the esc chain (file → tree →
    // diff → back). Only q and ? escape them.
    if (panel === "stack" && reviewMode === "file") {
      if (name === "q") return props.onQuit();
      if (name === "?") return overlays.help();
      if (name === "escape" || name === "h") return setReviewMode("tree");
      if (name === "c") return commentOnFileLine();
      if (name === "C") return openReviewOverlay();
      if (name === "j" || name === "down") return moveFileCursor(1);
      if (name === "k" || name === "up") return moveFileCursor(-1);
      if (name === "g") return scrollRef.current?.scrollTo(0);
      if (name === "G") return scrollRef.current?.scrollTo(scrollRef.current.scrollHeight);
      if (name === "space" || (key.ctrl && name === "d") || name === "pagedown")
        return scrollRef.current?.scrollBy(halfPage);
      if ((key.ctrl && name === "u") || name === "pageup")
        return scrollRef.current?.scrollBy(-halfPage);
      return;
    }
    if (panel === "stack" && reviewMode === "tree") {
      if (name === "q") return props.onQuit();
      if (name === "?") return overlays.help();
      if (name === "C") return openReviewOverlay();
      if (name === "escape") return setReviewMode("diff");
      if (name === "j" || name === "down")
        return setTreeSel((i) => clampSel(i + 1, treeRows.length));
      if (name === "k" || name === "up")
        return setTreeSel((i) => clampSel(i - 1, treeRows.length));
      if (name === "l" || name === "right" || name === "return" || name === "enter")
        return treeActivate();
      if (name === "h" || name === "left") return treeCollapse();
      return;
    }

    if (name === "q") return props.onQuit();
    if (name === "?") return overlays.help();
    if (name === "P") return props.onToggleProcs();
    // On the Processes tree, h/← collapse and l/→ expand (esc still goes back).
    if (panel === "procs" && (name === "h" || name === "left")) return toggleCollapse(false);
    if (panel === "procs" && (name === "l" || name === "right")) return toggleCollapse(true);
    if (name === "escape" || name === "h") {
      if (zoomed) return setZoomed(false); // unzoom before leaving the cockpit
      return props.onBack();
    }
    if (name === "w") return overlays.swap(slice, view.slice.active);
    // a attaches the terminal (launches the agent when autostart is configured);
    // C is the pending-review overlay (F2). Explicit agent launch stays on the
    // browser's C — the cockpit reuses the letter for review.
    if (name === "a") return props.onOpenTerm(slice, false);
    if (name === "C") return openReviewOverlay();
    if (name === "e") return overlays.editor(slice);
    if (name === "o") return overlays.editor(slice, selectedRepo);
    // F3: open the file-tree browser for the selected branch (diff sub-mode).
    if (name === "f" && panel === "stack" && stackReviewSupported) return openTree();
    if (
      panel === "stack" &&
      (name === "return" || name === "enter" || name === "l" || name === "right")
    ) {
      setDiffOpen(true);
      return;
    }
    // Enter on any other panel zooms the right pane full-width (enter/esc restores).
    if (name === "return" || name === "enter") {
      setZoomed((z) => !z);
      return;
    }
    // Panel cycle: tab forward, shift+tab/H backward (G9).
    if (name === "tab") {
      setPanel((p) => cyclePanel(p, key.shift ? -1 : 1));
      return;
    }
    if (name === "H") {
      setPanel((p) => cyclePanel(p, -1));
      return;
    }
    if (name >= "1" && name <= "4") {
      setPanel(PANEL_ORDER[Number(name) - 1]!);
      return;
    }
    // Refresh (G10) — plain r; ctrl+r stays CI-rerun on the PRs panel.
    if (name === "r" && !key.ctrl) return refreshCockpit();
    if (panel === "procs") {
      if (name === "s") return setProcSort((s) => nextSort(s));
      if (name === "x") return requestKill(false);
      if (name === "X") return requestKill(true);
    }
    if (panel === "prs") {
      if (name === "v") return toggleCiLog();
      if (key.ctrl && name === "r") return overlays.ciRerun(slice);
      if (name === "F") return overlays.fixCi(slice);
    }
    if (name === "j" || name === "down") return moveSel(1);
    if (name === "k" || name === "up") return moveSel(-1);
    // Scope cycling applies to the member branch only; other branches are always
    // shown vs their stack parent.
    if (name === "b" && panel === "stack" && onMemberBranch) {
      setScopeIdx((i) => (i + 1) % SCOPES.length);
      return;
    }
    if (name === "t" && panel === "stack") {
      setShowPatch((p) => !p);
      return;
    }
    // stack actions + slice mutations (overlays own the confirm / run flow).
    if (name === "R") return overlays.stack([slice], []);
    if (name === "s") return overlays.summary(slice, false);
    if (name === "S") return overlays.summary(slice, true);
    if (name === "d" && !key.ctrl) {
      if (view.slice.active)
        overlays.info("Cannot clear", `${slice} is live — swap back (w) first.`);
      else overlays.remove([slice]);
      return;
    }
    if (name === "y") return yankDiff();
    if (name === "Y") return overlays.yankPrStack(slice);
    if (name === "O") {
      const pr = focusedPr();
      if (pr?.url) overlays.openPr(pr.url);
      else overlays.info("Open PR", "No PR for the focused repo yet.");
      return;
    }
    if (name === "g") return scrollRef.current?.scrollTo(0);
    if (name === "G")
      return scrollRef.current?.scrollTo(scrollRef.current.scrollHeight);
    // Half-page scroll of the right pane: space (D7), ctrl+d/u, pgup/pgdn.
    if (name === "space") return scrollRef.current?.scrollBy(halfPage);
    if (key.ctrl && name === "d") return scrollRef.current?.scrollBy(halfPage);
    if (key.ctrl && name === "u") return scrollRef.current?.scrollBy(-halfPage);
    if (name === "pagedown") return scrollRef.current?.scrollBy(halfPage);
    if (name === "pageup") return scrollRef.current?.scrollBy(-halfPage);
  });

  // Clamp selections when data shrinks.
  useEffect(() => {
    setPrSel((i) => Math.max(0, Math.min(i, (view.prs?.length ?? 1) - 1)));
  }, [view.prs?.length]);
  useEffect(() => {
    setProcSel((i) => Math.max(0, Math.min(i, Math.max(0, procRows.length - 1))));
  }, [procRows.length]);
  useEffect(() => {
    setTreeSel((i) => clampSel(i, treeRows.length));
  }, [treeRows.length]);

  // A fresh slice starts with a clean process view (selection, kill state) and a
  // reset file-tree/file-review state.
  useEffect(() => {
    setProcSel(0);
    setCollapsed(new Set());
    setPendingKill(null);
    setKillStatus(null);
    setTreeChildren({});
    setTreeExpanded(new Set());
    setTreeSel(0);
    setOpenFile(null);
    setFileError(null);
    setFileCursor(0);
  }, [slice]);

  const leftW = Math.min(46, Math.floor(props.width / 2));
  const dividerW = Math.max(1, leftW - 2);

  const rightTitle = useMemo(() => {
    const arrow = ` ${glyph.arrow} `;
    switch (panel) {
      case "stack":
        if (reviewMode === "file")
          return `${selectedRepo}${arrow}${selectedBranch}${arrow}${openFile?.path ?? "file"}`;
        if (reviewMode === "tree")
          return `${selectedRepo}${arrow}${selectedBranch}${arrow}files`;
        if (onMemberBranch)
          return `${selectedRepo}${arrow}Changes · ${SCOPE_SHORT[scope]}`;
        return `${selectedRepo}${arrow}${selectedBranch}${arrow}vs ${branchDiff?.parent ?? "parent"}`;
      case "prs":
        return ciLog
          ? `${ciLog.repo}${arrow}CI log`
          : `${view.prs?.[prSel]?.repo ?? slice}${arrow}PR`;
      case "session":
        return `${slice}${arrow}Session`;
      case "procs":
        return `${slice}${arrow}Processes`;
    }
  }, [
    panel,
    selectedRepo,
    selectedBranch,
    onMemberBranch,
    reviewMode,
    openFile?.path,
    branchDiff?.parent,
    scope,
    view.prs,
    prSel,
    slice,
    ciLog,
  ]);

  const hints = useMemo(
    () =>
      cockpitHints(panel, {
        scope: SCOPE_SHORT[scope],
        showPatch,
        zoomed,
        killPending: !!pendingKill,
        reviewMode,
        onMember: onMemberBranch,
        stackReview: stackReviewSupported,
      }),
    [panel, scope, showPatch, zoomed, pendingKill, reviewMode, onMemberBranch, stackReviewSupported],
  );

  const headerTrailing = (
    <text wrapMode="none">
      {reviews.length > 0 ? (
        <>
          <span fg={color.candidate} attributes={BOLD}>
            {glyph.comment} {reviews.length}
          </span>
          <span> </span>
        </>
      ) : null}
      {view.slice.active ? <Badge state="live" label="LIVE · swapped in" bold /> : null}
      {view.slice.stale ? (
        <>
          {view.slice.active ? <span> </span> : null}
          <Badge state="stale" label="primary behind — refresh" />
        </>
      ) : null}
      <span fg={color.dim} attributes={DIM}>
        {"   esc back   ? help"}
      </span>
    </text>
  );

  // The rich (full-screen) diff shows the member branch's scoped slice diff, or —
  // when a non-member stack branch is selected — that branch's single-repo
  // diff-vs-parent.
  const diffViewRepos = onMemberBranch
    ? (diff?.repos ?? [])
    : branchDiff
      ? [
          {
            repo: branchDiff.repo,
            branch: branchDiff.branch,
            stat: branchDiff.stat,
            patch: branchDiff.patch,
            err: branchDiff.err,
          },
        ]
      : [];

  const cockpitSection =
    panel === "stack" && selectedBranch
      ? `${selectedRepo} ${glyph.arrow} ${selectedBranch}` +
        (reviewMode === "tree"
          ? ` ${glyph.arrow} files`
          : reviewMode === "file"
            ? ` ${glyph.arrow} ${openFile?.path?.split("/").pop() ?? "file"}`
            : "")
      : breadcrumbSection(panel, zoomed);

  if (diffOpen) {
    return (
      <DiffView
        enabled={enabled}
        repos={diffViewRepos}
        scope={onMemberBranch ? scope : "parent"}
        mode={diffMode}
        width={props.width}
        height={props.height}
        comments={reviews}
        onCycleScope={
          onMemberBranch ? () => setScopeIdx((i) => (i + 1) % SCOPES.length) : () => {}
        }
        onToggleMode={() => setDiffMode((m) => (m === "unified" ? "split" : "unified"))}
        onClose={() => setDiffOpen(false)}
        onQuit={props.onQuit}
        onComment={(target) => overlays.comment({ slice, ...target }, bumpReviews)}
        onReview={openReviewOverlay}
      />
    );
  }

  return (
    <box flexDirection="column" width="100%" height="100%">
      {/* header */}
      <Breadcrumb slice={slice} section={cockpitSection} trailing={headerTrailing} />
      <Divider color={theme.hairline} />
      {/* body */}
      <box flexDirection="row" flexGrow={1}>
        {zoomed ? null : (
          <box
            flexDirection="column"
            width={leftW}
            border={["right"]}
            borderColor={theme.hairline}
            paddingRight={1}
          >
            <StackSection
              view={view}
              focused={panel === "stack"}
              rows={stackRows}
              sel={stackSel}
              flexGrow={1}
            />
            <Divider width={dividerW} />
            <PrsSection view={view} focused={panel === "prs"} prSel={prSel} />
            <Divider width={dividerW} />
            <SessionSection view={view} focused={panel === "session"} lastLine={lastLine} />
            <Divider width={dividerW} />
            <ProcsSection procs={monitor.result} focused={panel === "procs"} />
          </box>
        )}
        <box flexGrow={1} flexDirection="column" paddingLeft={zoomed ? 0 : 1}>
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
              verticalScrollbarOptions={{
                showArrows: false,
                trackOptions: {
                  foregroundColor: theme.border,
                  backgroundColor: theme.surface,
                },
              }}
              horizontalScrollbarOptions={{ visible: false }}
            >
              {panel === "stack" ? (
                reviewMode === "file" ? (
                  <FileView
                    file={openFile}
                    error={fileError}
                    loading={fileLoading}
                    cursor={fileCursor}
                    marked={fileMarked}
                  />
                ) : reviewMode === "tree" ? (
                  <FileTree rows={treeRows} sel={treeSel} loading={treeLoading} />
                ) : onMemberBranch ? (
                  <DiffRight
                    diff={diff}
                    repo={selectedRepo}
                    scope={scope}
                    showPatch={showPatch}
                  />
                ) : (
                  <BranchDiffRight bd={branchDiff} repo={selectedRepo} showPatch={showPatch} />
                )
              ) : panel === "prs" ? (
                ciLog ? (
                  <CiLogRight state={ciLog} />
                ) : (
                  <PrDetailRight
                    view={view}
                    prSel={prSel}
                    comments={comments}
                    width={props.width - leftW}
                  />
                )
              ) : panel === "session" ? (
                <SessionRight view={view} lines={captureLines} />
              ) : (
                <ProcsRight
                  monitor={monitor}
                  rows={procRows}
                  procSel={procSel}
                  sort={procSort}
                  pendingKill={pendingKill}
                  killStatus={killStatus}
                />
              )}
            </scrollbox>
          </box>
        </box>
      </box>
      {/* footer */}
      <HintBar hints={hints} width={props.width - 1} />
    </box>
  );
}

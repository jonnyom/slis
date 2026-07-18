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
  CiLogResult,
  DiffResult,
  DiffScope,
  PrComment,
  PrStackEntry,
  ProcsResult,
  RpcClient,
} from "../rpc/types";
import type { SliceView } from "../state/derive";
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
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";
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
  repoSel,
  flexGrow,
}: {
  view: SliceView;
  focused: boolean;
  repoSel: number;
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
      {view.slice.members.map((m, i) => {
        const stack = view.show?.members.find((s) => s.repo === m.repo)?.stack;
        const selected = i === repoSel;
        return (
          <box key={m.repo} flexDirection="column">
            <text wrapMode="none">
              <span fg={color.cursorBar}>
                {selected && focused ? glyph.focusBar : " "}
              </span>
              <span fg={color.repoHeader} attributes={BOLD}>
                {m.repo}
              </span>
            </text>
            {stack && stack.length > 0 ? (
              stack.map((node) => {
                const isMember = node.name === m.branch;
                const c = node.trunk
                  ? color.synced
                  : node.needs_restack
                    ? color.restack
                    : isMember
                      ? color.white
                      : color.fg;
                const pad = "  ".repeat(Math.max(0, node.depth));
                return (
                  <text key={node.name} wrapMode="none">
                    <span fg={color.dim}>{"  " + pad}</span>
                    <span fg={c} attributes={isMember ? BOLD : 0}>
                      {node.name}
                    </span>
                    {node.trunk ? <span fg={color.synced}> [trunk]</span> : null}
                    {node.needs_restack ? (
                      <span fg={color.restack}> {glyph.dirty} restack</span>
                    ) : null}
                  </text>
                );
              })
            ) : (
              <text wrapMode="none">
                <span fg={color.dim}>  </span>
                <span fg={color.fg}>{m.branch}</span>
              </text>
            )}
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

  const [panel, setPanel] = useState<PanelId>("stack");
  const [repoSel, setRepoSel] = useState(0);
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
  const scrollRef = useRef<ScrollBoxRenderable>(null);

  const scope = SCOPES[scopeIdx]!;
  const selectedRepo = view.slice.members[repoSel]?.repo ?? "";

  const monitor = useProcMonitor(client, slice, panel === "procs");
  const comments = useComments(client, slice, panel === "prs");
  const sliceProcs = monitor.result?.slices[0]?.procs ?? [];
  const procRows = useMemo(
    () => flattenTree(buildProcTree(sliceProcs, procSort), collapsed),
    [sliceProcs, procSort, collapsed],
  );
  const captureLines = useCapture(client, slice, panel === "session", captureNonce);
  const lastLine = captureLines[captureLines.length - 1];

  // Load diff for the stack panel's right pane; refetch on slice/scope change.
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

  // Reset scroll to top whenever the right-pane content identity changes.
  useEffect(() => {
    scrollRef.current?.scrollTo(0);
  }, [panel, selectedRepo, scope, showPatch, prSel, ciLog?.repo, ciLog?.loading]);

  // Drop any open CI log when the focus moves off it (new PR, panel, or slice).
  useEffect(() => {
    setCiLog(null);
  }, [slice, panel, prSel]);

  const moveSel = (delta: number) => {
    if (panel === "stack")
      setRepoSel((i) =>
        Math.max(0, Math.min(view.slice.members.length - 1, i + delta)),
      );
    else if (panel === "prs")
      setPrSel((i) => Math.max(0, Math.min((view.prs?.length ?? 1) - 1, i + delta)));
    else if (panel === "procs")
      setProcSel((i) => Math.max(0, Math.min(procRows.length - 1, i + delta)));
  };

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
    if (panel === "session") setCaptureNonce((n) => n + 1);
    else props.onRefresh();
  };

  useKeyboard((key) => {
    if (!enabled) return;
    // While the full diff view is open it owns the keyboard.
    if (diffOpen) return;
    const name = key.name;

    // A pending kill confirmation captures input until answered.
    if (pendingKill) {
      if (name === "y" || name === "return" || name === "enter") return confirmKill();
      if (name === "n" || name === "escape") return setPendingKill(null);
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
    if (name === "a") return props.onOpenTerm(slice, false);
    if (name === "C") return props.onOpenTerm(slice, true);
    if (name === "e") return overlays.editor(slice);
    if (name === "o") return overlays.editor(slice, selectedRepo);
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
    if (name === "b" && panel === "stack") {
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

  // A fresh slice starts with a clean process view (selection, kill state).
  useEffect(() => {
    setProcSel(0);
    setCollapsed(new Set());
    setPendingKill(null);
    setKillStatus(null);
  }, [slice]);

  const leftW = Math.min(46, Math.floor(props.width / 2));
  const dividerW = Math.max(1, leftW - 2);

  const rightTitle = useMemo(() => {
    const arrow = ` ${glyph.arrow} `;
    switch (panel) {
      case "stack":
        return `${selectedRepo}${arrow}Changes · ${SCOPE_SHORT[scope]}`;
      case "prs":
        return ciLog
          ? `${ciLog.repo}${arrow}CI log`
          : `${view.prs?.[prSel]?.repo ?? slice}${arrow}PR`;
      case "session":
        return `${slice}${arrow}Session`;
      case "procs":
        return `${slice}${arrow}Processes`;
    }
  }, [panel, selectedRepo, scope, view.prs, prSel, slice, ciLog]);

  const hints = useMemo(
    () =>
      cockpitHints(panel, {
        scope: SCOPE_SHORT[scope],
        showPatch,
        zoomed,
        killPending: !!pendingKill,
      }),
    [panel, scope, showPatch, zoomed, pendingKill],
  );

  const headerTrailing = (
    <text wrapMode="none">
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

  if (diffOpen) {
    return (
      <DiffView
        enabled={enabled}
        repos={diff?.repos ?? []}
        scope={scope}
        mode={diffMode}
        width={props.width}
        height={props.height}
        onCycleScope={() => setScopeIdx((i) => (i + 1) % SCOPES.length)}
        onToggleMode={() => setDiffMode((m) => (m === "unified" ? "split" : "unified"))}
        onClose={() => setDiffOpen(false)}
        onQuit={props.onQuit}
      />
    );
  }

  return (
    <box flexDirection="column" width="100%" height="100%">
      {/* header */}
      <Breadcrumb
        slice={slice}
        section={breadcrumbSection(panel, zoomed)}
        trailing={headerTrailing}
      />
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
              repoSel={repoSel}
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
              scrollbarOptions={{ visible: true }}
            >
              {panel === "stack" ? (
                <DiffRight
                  diff={diff}
                  repo={selectedRepo}
                  scope={scope}
                  showPatch={showPatch}
                />
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

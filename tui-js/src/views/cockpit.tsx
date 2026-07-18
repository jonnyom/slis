// Cockpit view: the lazygit-style detail screen for a single slice.
//
//   slis ▸ checkout            ● LIVE — swapped in · [w] back   [esc] back ? help
//   ┌ 1 Repos & Stack ┐ ┌ right pane (driven by focused panel) ──────────────┐
//   ┌ 2 PRs           ┐ │  Stack → diff (b: scope, t: stat/patch)             │
//   ┌ 3 Session       ┐ │  PRs → PR detail                                    │
//   ┌ 4 Processes     ┐ │  Session → capture tail + worktrees                 │
//   └ footer hints ───┘ │  Processes → process table                         │

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
  DiffResult,
  DiffScope,
  ProcsResult,
  RpcClient,
} from "../rpc/types";
import type { SliceView } from "../state/derive";
import type { OverlayApi } from "../overlays/useOverlays";
import { color, glyph, sessionBadge, sessionLabel } from "../theme";
import { Panel } from "../components/panel";
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

type PanelId = "stack" | "prs" | "session" | "procs";
const PANEL_ORDER: PanelId[] = ["stack", "prs", "session", "procs"];
const SCOPES: DiffScope[] = ["working", "parent", "trunk"];
const SCOPE_LABEL: Record<DiffScope, string> = {
  working: "working tree (dirty)",
  parent: "vs parent",
  trunk: "vs trunk",
};

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
  onQuit: () => void;
}

function useCapture(
  client: RpcClient,
  slice: string,
  tick: boolean,
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
  }, [client, slice, tick]);
  return lines;
}

// ── left panels ──────────────────────────────────────────────────────────────

function StackPanel({
  view,
  focused,
  repoSel,
  height,
}: {
  view: SliceView;
  focused: boolean;
  repoSel: number;
  height: number;
}): ReactNode {
  return (
    <Panel title="Repos & Stack" index={1} focused={focused} height={height}>
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
              stack.map((n) => {
                const isMember = n.name === m.branch;
                const c = n.trunk
                  ? color.synced
                  : n.needs_restack
                    ? color.restack
                    : isMember
                      ? color.white
                      : color.fg;
                const pad = "  ".repeat(Math.max(0, n.depth));
                return (
                  <text key={n.name} wrapMode="none">
                    <span fg={color.dim}>{"  " + pad}</span>
                    <span fg={c} attributes={isMember ? BOLD : 0}>
                      {n.name}
                    </span>
                    {n.trunk ? <span fg={color.synced}> [trunk]</span> : null}
                    {n.needs_restack ? (
                      <span fg={color.restack}> ⚠ restack</span>
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

function PrsPanel({
  view,
  focused,
  prSel,
  height,
}: {
  view: SliceView;
  focused: boolean;
  prSel: number;
  height: number;
}): ReactNode {
  const prs = view.prs ?? [];
  return (
    <Panel title="PRs" index={2} focused={focused} height={height}>
      {prs.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          loading…
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
                  {pr.review_decision === "APPROVED" ? (
                    <span fg={color.synced}> ✓</span>
                  ) : pr.review_decision === "CHANGES_REQUESTED" ? (
                    <span fg={color.missing}> ✗</span>
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

function SessionPanel({
  view,
  focused,
  lastLine,
  height,
}: {
  view: SliceView;
  focused: boolean;
  lastLine: string | undefined;
  height: number;
}): ReactNode {
  const badge = sessionBadge(view.status);
  return (
    <Panel title="Session" index={3} focused={focused} height={height}>
      <text wrapMode="none">
        <span fg={badge.color} attributes={BOLD}>
          {badge.glyph}
        </span>
        <span fg={color.fg}> {sessionLabel(view.status)}</span>
      </text>
      {lastLine ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          {lastLine}
        </text>
      ) : null}
    </Panel>
  );
}

function ProcsPanel({
  procs,
  focused,
  height,
}: {
  procs: ProcsResult | null;
  focused: boolean;
  height: number;
}): ReactNode {
  const slice = procs?.slices[0];
  const total = slice?.totalCPU ?? 0;
  const over = total > SLICE_CPU_WARN;
  return (
    <Panel title="Processes" index={4} focused={focused} height={height}>
      {!procs ? (
        <text fg={color.dim} attributes={DIM}>
          sampling…
        </text>
      ) : !slice || slice.procs.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          no session / no processes
        </text>
      ) : (
        <>
          {slice.procs.slice(0, 2).map((p) => (
            <text key={p.pid} wrapMode="none">
              <span fg={color.live}>● </span>
              <span fg={color.fg}>{p.cmd}</span>
              <span fg={color.dim}> {p.cpu.toFixed(0)}%</span>
            </text>
          ))}
          <text wrapMode="none">
            <span fg={over ? color.restack : color.dim}>
              Σ {total.toFixed(0)}%
            </span>
            {over ? <span fg={color.restack} attributes={BOLD}> ⚠</span> : null}
          </text>
        </>
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

function PrDetailRight({ view, prSel }: { view: SliceView; prSel: number }): ReactNode {
  const pr = (view.prs ?? [])[prSel];
  if (!pr) return <text fg={color.dim}>no PR selected</text>;
  if (pr.number === undefined) {
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
    </>
  );
}

function SessionRight({ lines }: { lines: string[] }): ReactNode {
  if (lines.length === 0) {
    return (
      <text fg={color.dim} attributes={DIM}>
        (no session output — attach with the CLI to start one)
      </text>
    );
  }
  return (
    <>
      {lines.map((l, i) => (
        <text key={i} fg={color.fg} wrapMode="none">
          {l === "" ? " " : l}
        </text>
      ))}
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
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [diffOpen, setDiffOpen] = useState(false);
  const [diffMode, setDiffMode] = useState<DiffMode>("unified");
  const scrollRef = useRef<ScrollBoxRenderable>(null);

  const scope = SCOPES[scopeIdx]!;
  const selectedRepo = view.slice.members[repoSel]?.repo ?? "";

  const monitor = useProcMonitor(client, slice, panel === "procs");
  const sliceProcs = monitor.result?.slices[0]?.procs ?? [];
  const procRows = useMemo(
    () => flattenTree(buildProcTree(sliceProcs, procSort), collapsed),
    [sliceProcs, procSort, collapsed],
  );
  const captureLines = useCapture(client, slice, panel === "session");
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
  }, [panel, selectedRepo, scope, showPatch, prSel]);

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
    if (name === "escape" || name === "h") return props.onBack();
    if (name === "w") return overlays.swap(slice, view.slice.active);
    if (name === "a") return props.onOpenTerm(slice, false);
    if (name === "C") return props.onOpenTerm(slice, true);
    if (
      panel === "stack" &&
      (name === "return" || name === "enter" || name === "l" || name === "right")
    ) {
      setDiffOpen(true);
      return;
    }
    if (name === "tab") {
      setPanel((p) => PANEL_ORDER[(PANEL_ORDER.indexOf(p) + 1) % PANEL_ORDER.length]!);
      return;
    }
    if (name >= "1" && name <= "4") {
      setPanel(PANEL_ORDER[Number(name) - 1]!);
      return;
    }
    if (panel === "procs") {
      if (name === "s") return setProcSort((s) => nextSort(s));
      if (name === "x") return requestKill(false);
      if (name === "X") return requestKill(true);
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
    if (key.ctrl && name === "d") return scrollRef.current?.scrollBy(10);
    if (key.ctrl && name === "u") return scrollRef.current?.scrollBy(-10);
    if (name === "pagedown") return scrollRef.current?.scrollBy(10);
    if (name === "pageup") return scrollRef.current?.scrollBy(-10);
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

  const leftW = Math.min(38, Math.floor(props.width / 2));
  const bodyH = props.height - 2; // header + footer
  const sessionH = 4;
  const procsH = 4;
  const prsH = Math.max(4, Math.min(9, (view.slice.members.length || 1) + 3));
  const stackH = Math.max(4, bodyH - sessionH - procsH - prsH);

  const rightTitle = useMemo(() => {
    switch (panel) {
      case "stack":
        return `${selectedRepo} · Changes`;
      case "prs":
        return `${view.prs?.[prSel]?.repo ?? slice} · PR`;
      case "session":
        return `Session · ${slice}`;
      case "procs":
        return `Processes · ${slice}`;
    }
  }, [panel, selectedRepo, view.prs, prSel, slice]);

  const footer = useMemo(() => {
    const common = "w swap · R stack · s/S summary · y/Y yank · d clear · esc back";
    switch (panel) {
      case "stack":
        return `tab panel · j/k repo · enter rich diff · b scope:${SCOPE_LABEL[scope]} · t ${showPatch ? "stat" : "patch"} · a/C term · ${common}`;
      case "prs":
        return `tab panel · j/k pr · O open PR · a/C term · ${common}`;
      case "session":
        return `tab panel · a/C term · ${common}`;
      case "procs":
        return "tab panel · j/k proc · h/l fold · s sort · x/X kill · a/C term · w swap · d clear · esc back";
    }
  }, [panel, scope, showPatch]);

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
      <box flexDirection="row" justifyContent="space-between">
        <text wrapMode="none">
          <span fg={color.title} attributes={BOLD}>
            slis {glyph.filterMarker} {slice}
          </span>
          {view.slice.active ? (
            <span fg={color.live}>
              {"   "}
              {glyph.live} LIVE — swapped in · [w] swap back
            </span>
          ) : null}
          {view.slice.stale ? (
            <span fg={color.wait}>  ⚠ primary behind tip — run slis refresh</span>
          ) : null}
        </text>
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          [esc] back  ? help
        </text>
      </box>
      {/* body */}
      <box flexDirection="row" flexGrow={1}>
        <box flexDirection="column" width={leftW}>
          <StackPanel
            view={view}
            focused={panel === "stack"}
            repoSel={repoSel}
            height={stackH}
          />
          <PrsPanel view={view} focused={panel === "prs"} prSel={prSel} height={prsH} />
          <SessionPanel
            view={view}
            focused={panel === "session"}
            lastLine={lastLine}
            height={sessionH}
          />
          <ProcsPanel procs={monitor.result} focused={panel === "procs"} height={procsH} />
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
            >
              {panel === "stack" ? (
                <DiffRight
                  diff={diff}
                  repo={selectedRepo}
                  scope={scope}
                  showPatch={showPatch}
                />
              ) : panel === "prs" ? (
                <PrDetailRight view={view} prSel={prSel} />
              ) : panel === "session" ? (
                <SessionRight lines={captureLines} />
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
      <text wrapMode="none" fg={color.dim} attributes={DIM}>
        {footer}
      </text>
    </box>
  );
}

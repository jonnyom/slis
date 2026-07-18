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
import { color, glyph, sessionBadge, sessionLabel } from "../theme";
import { Panel } from "../components/panel";
import { DiffPane } from "../components/diffpane";
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";

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
  width: number;
  height: number;
  onBack: () => void;
  onSwap: (slice: string) => void;
  onToggleHelp: () => void;
  onQuit: () => void;
}

function useProcs(client: RpcClient, slice: string, active: boolean): ProcsResult | null {
  const [procs, setProcs] = useState<ProcsResult | null>(null);
  useEffect(() => {
    if (!active) return;
    let live = true;
    const load = () =>
      client.procs(slice).then((r) => live && setProcs(r), () => {});
    load();
    const id = setInterval(load, 2000);
    return () => {
      live = false;
      clearInterval(id);
    };
  }, [client, slice, active]);
  return procs;
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
            <span fg={slice.totalCPU > 80 ? color.restack : color.dim}>
              Σ {slice.totalCPU.toFixed(0)}%
            </span>
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
  procs,
  procSel,
}: {
  procs: ProcsResult | null;
  procSel: number;
}): ReactNode {
  const slice = procs?.slices[0];
  if (!procs) return <text fg={color.dim} attributes={DIM}>sampling…</text>;
  if (!slice || slice.procs.length === 0)
    return (
      <text fg={color.dim} attributes={DIM}>
        no tmux session / no processes
      </text>
    );
  return (
    <>
      <text wrapMode="none">
        <span fg={color.dim}>{"  PID".padEnd(9)}</span>
        <span fg={color.dim}>{"CPU%".padStart(6)}</span>
        <span fg={color.dim}>{"MEM MB".padStart(9)}</span>
        <span fg={color.dim}>  CMD</span>
      </text>
      {slice.procs.map((p, i) => {
        const selected = i === procSel;
        return (
          <text key={p.pid} wrapMode="none" attributes={selected ? BOLD : 0}>
            <span fg={color.cursorBar}>{selected ? glyph.focusBar + " " : "  "}</span>
            <span fg={selected ? color.white : color.fg}>
              {String(p.pid).padEnd(7)}
            </span>
            <span fg={p.cpu > 50 ? color.restack : color.fg}>
              {p.cpu.toFixed(1).padStart(6)}
            </span>
            <span fg={color.fg}>{p.mem.toFixed(1).padStart(9)}</span>
            <span fg={color.fg}>  {p.cmd}</span>
          </text>
        );
      })}
    </>
  );
}

// ── cockpit ──────────────────────────────────────────────────────────────────

export function Cockpit(props: CockpitProps): ReactNode {
  const { view, client, enabled } = props;
  const slice = view.slice.name;

  const [panel, setPanel] = useState<PanelId>("stack");
  const [repoSel, setRepoSel] = useState(0);
  const [prSel, setPrSel] = useState(0);
  const [procSel, setProcSel] = useState(0);
  const [scopeIdx, setScopeIdx] = useState(0);
  const [showPatch, setShowPatch] = useState(false);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const scrollRef = useRef<ScrollBoxRenderable>(null);

  const scope = SCOPES[scopeIdx]!;
  const selectedRepo = view.slice.members[repoSel]?.repo ?? "";

  const procs = useProcs(client, slice, panel === "procs");
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
      setProcSel((i) =>
        Math.max(0, Math.min((procs?.slices[0]?.procs.length ?? 1) - 1, i + delta)),
      );
  };

  useKeyboard((key) => {
    if (!enabled) return;
    const name = key.name;
    if (name === "q") return props.onQuit();
    if (name === "?") return props.onToggleHelp();
    if (name === "escape" || name === "h") return props.onBack();
    if (name === "w") return props.onSwap(slice);
    if (name === "tab") {
      setPanel((p) => PANEL_ORDER[(PANEL_ORDER.indexOf(p) + 1) % PANEL_ORDER.length]!);
      return;
    }
    if (name >= "1" && name <= "4") {
      setPanel(PANEL_ORDER[Number(name) - 1]!);
      return;
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
  const procCount = procs?.slices[0]?.procs.length ?? 0;
  useEffect(() => {
    setProcSel((i) => Math.max(0, Math.min(i, Math.max(0, procCount - 1))));
  }, [procCount]);

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
    switch (panel) {
      case "stack":
        return `tab panel · j/k repo · b scope:${SCOPE_LABEL[scope]} · t ${showPatch ? "stat" : "patch"} · ^d/^u scroll · w swap · esc back`;
      case "prs":
        return "tab panel · j/k pr · w swap · esc back";
      case "session":
        return "tab panel · w swap · esc back";
      case "procs":
        return "tab panel · j/k proc · w swap · esc back";
    }
  }, [panel, scope, showPatch]);

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
          <ProcsPanel procs={procs} focused={panel === "procs"} height={procsH} />
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
                <ProcsRight procs={procs} procSel={procSel} />
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

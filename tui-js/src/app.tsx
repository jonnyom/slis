// Top-level app: owns the RPC client lifecycle, workspace data, live session
// badges, view routing (browser ⇄ cockpit) and the overlay layer (useOverlays).

import { useRenderer, useTerminalDimensions } from "@opentui/react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createRpcClient } from "./rpc";
import type {
  ConflictsResult,
  HelloResult,
  LsResult,
  PrStackEntry,
  RpcClient,
  SessionStatus,
  ShowResult,
} from "./rpc/types";
import type { SliceView } from "./state/derive";
import { color } from "./theme";
import { Browser } from "./views/browser";
import { Cockpit } from "./views/cockpit";
import { AllSlicesProcOverlay } from "./components/procoverlay";
import { useOverlays } from "./overlays/useOverlays";
import { BOLD, DIM } from "./components/ui";
import { TermManager } from "./term/manager";
import { TerminalLayer, type TabEntry } from "./term/tabs";
import { tmuxAvailable, type TermMember } from "./term/tmux";
import type { TermSessionOpts } from "./term/session";
import { availableEditors } from "./editor/detect";
import { bulkLoadPlan, type BulkPhase } from "./state/bulkload";
import { BulkLoadOverlay } from "./components/bulkload";

export function App(): ReactNode {
  const renderer = useRenderer();
  const { width, height } = useTerminalDimensions();

  const clientRef = useRef<RpcClient | null>(null);
  if (!clientRef.current) clientRef.current = createRpcClient();
  const client = clientRef.current;

  const [hello, setHello] = useState<HelloResult | null>(null);
  const [ls, setLs] = useState<LsResult | null>(null);
  const [statuses, setStatuses] = useState<Record<string, SessionStatus>>({});
  const [prStacks, setPrStacks] = useState<Record<string, PrStackEntry[]>>({});
  const [shows, setShows] = useState<Record<string, ShowResult>>({});
  const [conflicts, setConflicts] = useState<ConflictsResult | null>(null);
  const [connected, setConnected] = useState(true);

  const [view, setView] = useState<"browser" | "cockpit">("browser");
  const [current, setCurrent] = useState<string | null>(null);
  const [procsOpen, setProcsOpen] = useState(false);

  // Embedded terminal session tabs.
  const managerRef = useRef<TermManager | null>(null);
  if (!managerRef.current) managerRef.current = new TermManager();
  const manager = managerRef.current;
  const [tabs, setTabs] = useState<TabEntry[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [termMode, setTermMode] = useState(false);

  const bulkPhaseRef = useRef<BulkPhase>("unprompted");
  const loadedRef = useRef<Set<string>>(new Set());
  const [bulkPromptCount, setBulkPromptCount] = useState<number | null>(null);

  const loadSlice = useCallback(
    (name: string) => {
      loadedRef.current.add(name);
      client.prStack(name).then(
        (prs) => setPrStacks((m) => ({ ...m, [name]: prs })),
        () => {},
      );
      client.show(name).then(
        (show) => setShows((m) => ({ ...m, [name]: show })),
        () => {},
      );
    },
    [client],
  );

  const refresh = useCallback(() => {
    // ls first so the browser paints fast; the sidecar caps subprocess work at
    // 4 in flight, so the expensive fan-out (conflicts + per-slice PR/stack)
    // must wait behind ls rather than starve it.
    client.hello().then(setHello, () => {});
    client.status().then(
      (rows) => setStatuses(Object.fromEntries(rows.map((r) => [r.slice, r.status]))),
      () => {},
    );
    client.ls().then((res) => {
      setLs(res);
      client.conflicts().then(setConflicts, () => {});
      const plan = bulkLoadPlan(res.slices.length, bulkPhaseRef.current);
      if (plan.prompt) {
        setBulkPromptCount(res.slices.length);
        return;
      }
      setBulkPromptCount(null);
      if (plan.fanOut) {
        loadedRef.current = new Set(res.slices.map((s) => s.name));
        for (const s of res.slices) loadSlice(s.name);
      }
    }, () => {});
  }, [client, loadSlice]);

  const applyBulkChoice = useCallback(
    (phase: BulkPhase) => {
      bulkPhaseRef.current = phase;
      setBulkPromptCount(null);
      refresh();
    },
    [refresh],
  );

  const onFocusSlice = useCallback(
    (name: string) => {
      if (bulkPhaseRef.current === "lazy" && !loadedRef.current.has(name)) loadSlice(name);
    },
    [loadSlice],
  );

  // Initial load + subscriptions.
  useEffect(() => {
    refresh();
    const offEvent = client.onSessionEvent((e) =>
      setStatuses((m) => ({ ...m, [e.slice]: e.status })),
    );
    const offConn = client.onConnectionChange((c) => {
      setConnected(c);
      if (c) refresh(); // resync after a reconnect
    });
    return () => {
      offEvent();
      offConn();
    };
  }, [client, refresh]);

  const quit = useCallback(() => {
    manager.detachAll(); // drop every tmux client — sessions keep running
    client.close();
    renderer.destroy();
    process.exit(0);
  }, [client, renderer, manager]);

  // Editors found on PATH (probed once). Combined with the configured editor
  // from `hello`, the overlay layer decides whether to open directly or prompt.
  const editorList = useMemo(() => availableEditors((b) => !!Bun.which(b)), []);

  const overlays = useOverlays({
    refresh,
    conflicts,
    view,
    height,
    client,
    editors: editorList,
    configuredEditor: hello?.sessions.editor,
  });

  // Build per-slice view records.
  const views: SliceView[] = useMemo(() => {
    if (!ls) return [];
    return ls.slices.map((slice) => ({
      slice,
      status: statuses[slice.name] ?? "none",
      prs: prStacks[slice.name],
      show: shows[slice.name],
    }));
  }, [ls, statuses, prStacks, shows]);

  const currentView = useMemo(
    () => views.find((v) => v.slice.name === current),
    [views, current],
  );

  const onEnter = useCallback(
    (slice: string) => {
      if (bulkPhaseRef.current === "lazy" && !loadedRef.current.has(slice)) loadSlice(slice);
      setCurrent(slice);
      setView("cockpit");
    },
    [loadSlice],
  );

  // ── embedded terminal tabs ────────────────────────────────────────────────

  // Build the attach options for a slice from live workspace data. The harness,
  // agent and layout come from the workspace's sessions config (surfaced by the
  // `hello` RPC); an older sidecar without that field falls back to the Go-TUI
  // defaults. Autostart is OR'd into launchAgent so a plain attach launches the
  // agent when configured, mirroring the Go TUI's attach.
  const buildTermOpts = useCallback(
    (slice: string, launchAgent: boolean): TermSessionOpts | null => {
      const v = views.find((x) => x.slice.name === slice);
      if (!v) return null;
      const wsRoot = hello?.workspaceRoot ?? "";
      const sessions = hello?.sessions;
      const members: TermMember[] = v.slice.members.map((m) => ({
        repo: m.repo,
        branch: m.branch,
        worktreePath: m.worktree_path,
      }));
      return {
        slice,
        members,
        active: v.slice.active,
        wsRoot,
        sessionOpts: { root: wsRoot, layout: sessions?.layout ?? "" },
        launchAgent: launchAgent || (sessions?.autostart ?? false),
        agent: sessions?.agent || "claude",
        harness: sessions?.harness || "claude",
      };
    },
    [views, hello],
  );

  const openTerm = useCallback(
    (slice: string, launchAgent: boolean) => {
      if (!tmuxAvailable()) {
        overlays.info(
          "Terminal unavailable",
          "tmux is not on PATH — install tmux to use session tabs.",
        );
        return;
      }
      setTabs((prev) => {
        if (prev.some((t) => t.slice === slice)) return prev; // reuse open tab
        const opts = buildTermOpts(slice, launchAgent);
        return opts ? [...prev, { slice, opts }] : prev;
      });
      setActiveTab(slice);
      setTermMode(true);
    },
    [buildTermOpts, overlays],
  );

  const closeTab = useCallback((slice: string) => {
    setTabs((prev) => {
      const next = prev.filter((t) => t.slice !== slice);
      setActiveTab((cur) => {
        if (cur !== slice) return cur;
        return next.length > 0 ? next[next.length - 1]!.slice : null;
      });
      if (next.length === 0) setTermMode(false);
      return next;
    });
  }, []);

  if (!ls) {
    return (
      <box width="100%" height="100%" alignItems="center" justifyContent="center">
        <text fg={color.dim} attributes={DIM}>
          {connected ? "loading workspace…" : "connecting to slis rpc…"}
        </text>
      </box>
    );
  }

  const bulkPromptOpen = bulkPromptCount !== null;
  const overlayEnabled = !overlays.active && !procsOpen && !bulkPromptOpen;

  return (
    <box width="100%" height="100%">
      {view === "browser" || !currentView ? (
        <Browser
          enabled={overlayEnabled && !termMode && view === "browser"}
          client={client}
          version={hello?.version ?? "?"}
          workspaceRoot={hello?.workspaceRoot ?? ""}
          views={views}
          ls={ls}
          conflicts={conflicts}
          overlays={overlays}
          width={width}
          height={height}
          onEnter={onEnter}
          onOpenTerm={openTerm}
          onFocusSlice={onFocusSlice}
          onRefresh={refresh}
          onToggleProcs={() => setProcsOpen(true)}
          onQuit={quit}
        />
      ) : (
        <Cockpit
          enabled={overlayEnabled && !termMode}
          client={client}
          view={currentView}
          overlays={overlays}
          width={width}
          height={height}
          onBack={() => setView("browser")}
          onOpenTerm={openTerm}
          onToggleProcs={() => setProcsOpen(true)}
          onQuit={quit}
        />
      )}

      <TerminalLayer
        tabs={tabs}
        active={activeTab}
        focused={termMode}
        statuses={statuses}
        width={width}
        height={height}
        manager={manager}
        onBack={() => setTermMode(false)}
        onExit={closeTab}
      />

      {!connected ? (
        <box position="absolute" top={0} left={0} width="100%">
          <text fg={color.missing} attributes={BOLD}>
            {"  "}⚠ sidecar disconnected — reconnecting…
          </text>
        </box>
      ) : null}

      {overlays.node}
      {procsOpen ? (
        <AllSlicesProcOverlay
          client={client}
          enabled={procsOpen}
          onClose={() => setProcsOpen(false)}
        />
      ) : null}
      {bulkPromptOpen ? (
        <BulkLoadOverlay
          count={bulkPromptCount ?? 0}
          enabled={bulkPromptOpen && !termMode}
          onLoadAll={() => applyBulkChoice("all")}
          onLazy={() => applyBulkChoice("lazy")}
        />
      ) : null}
    </box>
  );
}

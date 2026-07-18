// Top-level app: owns the RPC client lifecycle, workspace data, live session
// badges, view routing (browser ⇄ cockpit) and the overlay layer (useOverlays).

import { useRenderer, useTerminalDimensions } from "@opentui/react";
import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createRpcClient } from "./rpc";
import type {
  AgentSpec,
  ConflictsResult,
  HelloResult,
  LsResult,
  PrStackEntry,
  RpcClient,
  SessionStatus,
  ShowResult,
} from "./rpc/types";
import type { SliceView } from "./state/derive";
import { theme } from "./theme";
import { Browser } from "./views/browser";
import { Cockpit } from "./views/cockpit";
import { AllSlicesProcOverlay } from "./components/procoverlay";
import { useOverlays, type OverlayApi } from "./overlays/useOverlays";
import { DIM } from "./components/ui";
import { TermManager } from "./term/manager";
import { TerminalLayer, tabKey, type TabEntry } from "./term/tabs";
import { tmuxAvailable, type TermMember } from "./term/tmux";
import type { TermSessionOpts } from "./term/session";
import { pickableAgents, agentCmdline } from "./term/agentpick";
import { availableEditors } from "./editor/detect";
import { bulkLoadPlan, type BulkPhase } from "./state/bulkload";
import { BulkLoadOverlay } from "./components/bulkload";
import { useToasts, ToastLayer } from "./components/toast";
import { createSlice } from "./rpc/mutate";
import {
  createBusyLabel,
  createReducer,
  initialCreateState,
} from "./state/create";
import { tickPlan } from "./state/tick";

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
  const nextCmdIdRef = useRef(0);

  const bulkPhaseRef = useRef<BulkPhase>("unprompted");
  const loadedRef = useRef<Set<string>>(new Set());
  const [bulkPromptCount, setBulkPromptCount] = useState<number | null>(null);

  // Transient toasts (spec §3.5) + non-blocking create (spec D2).
  const { toasts, push: pushToast, dismiss: dismissToast } = useToasts();
  const [createState, dispatchCreate] = useReducer(createReducer, initialCreateState);

  // The slice the browser has focused (for the lazy-mode background tick, G7).
  const browserFocusRef = useRef<string | null>(null);

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

  const refresh = useCallback(
    (onDone?: () => void) => {
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
        onDone?.();
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
    },
    [client, loadSlice],
  );

  // Manual refresh (`r`) confirms with a toast once ls returns.
  const manualRefresh = useCallback(() => {
    refresh(() => pushToast("Refreshed workspace", "ci-pass"));
  }, [refresh, pushToast]);

  // Targeted background refresh for the 30s tick (G7): PR/stack for the planned
  // slices + conflicts + session statuses, without re-running ls (so it never
  // re-triggers the bulk-load prompt or resets the lazy phase).
  const tickRefresh = useCallback(
    (sliceNames: string[]) => {
      client.status().then(
        (rows) => setStatuses(Object.fromEntries(rows.map((r) => [r.slice, r.status]))),
        () => {},
      );
      client.conflicts().then(setConflicts, () => {});
      for (const name of sliceNames) loadSlice(name);
    },
    [client, loadSlice],
  );

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
      browserFocusRef.current = name;
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

  // 30s background refresh tick (parity gap G7). The gating context is mirrored
  // into a ref each render so the interval is created once (not reset on every
  // navigation / status change). `tickPlan` decides whether to run and which
  // slices — paused while a PTY tab or bulk-load prompt is up, focused-slice
  // only in lazy mode, everything otherwise.
  const tickCtxRef = useRef({
    paused: false,
    phase: bulkPhaseRef.current,
    focusedSlice: null as string | null,
    slices: [] as string[],
  });
  tickCtxRef.current = {
    paused: termMode || bulkPromptCount !== null,
    phase: bulkPhaseRef.current,
    focusedSlice: view === "cockpit" ? current : browserFocusRef.current,
    slices: ls?.slices.map((s) => s.name) ?? [],
  };
  useEffect(() => {
    const id = setInterval(() => {
      const plan = tickPlan(tickCtxRef.current);
      if (plan.run) tickRefresh(plan.slices);
    }, 30_000);
    return () => clearInterval(id);
  }, [tickRefresh]);

  const quit = useCallback(() => {
    manager.detachAll(); // drop every tmux client — sessions keep running
    client.close();
    renderer.destroy();
    process.exit(0);
  }, [client, renderer, manager]);

  // Editors found on PATH (probed once). Combined with the configured editor
  // from `hello`, the overlay layer decides whether to open directly or prompt.
  const editorList = useMemo(() => availableEditors((b) => !!Bun.which(b)), []);

  // Open an interactive mutation (submit/sync/merge/adopt/fix-ci) in a PTY tab
  // so it gets a real TTY — the overlay layer routes these here instead of the
  // captured runner. The tab title is the command; it stays open after exit so
  // the user can read the outcome and close it (ctrl+q → refresh).
  const openCommandTab = useCallback((argv: string[], title: string) => {
    const id = `cmd:${nextCmdIdRef.current++}:${title}`;
    setTabs((prev) => [...prev, { kind: "command", id, title, argv, exited: false }]);
    setActiveTab(id);
    setTermMode(true);
  }, []);

  // Non-blocking create (spec D2): run in the background with an ambient header
  // spinner; success → toast, failure → Result overlay (via the overlays ref,
  // which is assigned just below to break the useOverlays ⇄ startCreate cycle).
  const overlaysRef = useRef<OverlayApi | null>(null);
  const startCreate = useCallback(
    (name: string) => {
      dispatchCreate({ type: "start", name });
      createSlice(name).then(
        (res) => {
          dispatchCreate({ type: "finish" });
          if (res.code === 0) {
            pushToast(`Created ${name}`, "ci-pass");
            refresh();
          } else {
            const body =
              (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)";
            overlaysRef.current?.error(`Create ${name} — failed`, body);
          }
        },
        (err) => {
          dispatchCreate({ type: "finish" });
          overlaysRef.current?.error(`Create ${name} — failed`, String(err));
        },
      );
    },
    [pushToast, refresh],
  );

  const overlays = useOverlays({
    refresh,
    conflicts,
    view,
    height,
    client,
    editors: editorList,
    configuredEditor: hello?.sessions.editor,
    runInteractive: openCommandTab,
    toast: pushToast,
    startCreate,
  });
  overlaysRef.current = overlays;

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
    (slice: string, launchAgent: boolean, choice?: AgentSpec): TermSessionOpts | null => {
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
        agent: choice ? agentCmdline(choice.cmd) : sessions?.agent || "claude",
        harness: sessions?.harness || "claude",
        agentLabel: choice?.name,
      };
    },
    [views, hello],
  );

  // Open (or reuse) the slice's terminal tab, attaching a tmux client and — when
  // launching an agent — running the picked (or default) agent in it.
  const launchTermTab = useCallback(
    (slice: string, launchAgent: boolean, choice?: AgentSpec) => {
      setTabs((prev) => {
        if (prev.some((t) => t.kind === "session" && t.slice === slice)) return prev; // reuse open tab
        const opts = buildTermOpts(slice, launchAgent, choice);
        return opts ? [...prev, { kind: "session", slice, opts }] : prev;
      });
      setActiveTab(slice);
      setTermMode(true);
    },
    [buildTermOpts],
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
      // With more than one configured agent, a launch (C / autostart) first asks
      // which one; a single agent (or an older sidecar) keeps the direct path.
      if (launchAgent) {
        const choices = pickableAgents(hello?.agents);
        if (choices.length > 1) {
          overlays.agentPicker(slice, choices, (choice) => launchTermTab(slice, true, choice));
          return;
        }
      }
      launchTermTab(slice, launchAgent);
    },
    [launchTermTab, overlays, hello],
  );

  // Remove a tab and re-point the active tab / term mode. When a *command* tab
  // closes, run the post-mutation refresh — the same resync the captured path
  // does after a mutation completes.
  const closeTab = useCallback(
    (key: string) => {
      let wasCommand = false;
      setTabs((prev) => {
        wasCommand = prev.some((t) => t.kind === "command" && t.id === key);
        const next = prev.filter((t) => tabKey(t) !== key);
        setActiveTab((cur) => {
          if (cur !== key) return cur;
          return next.length > 0 ? tabKey(next[next.length - 1]!) : null;
        });
        if (next.length === 0) setTermMode(false);
        return next;
      });
      if (wasCommand) refresh();
    },
    [refresh],
  );

  // A command process exited: mark its tab so the tab bar shows the status glyph
  // and the back key knows it can be closed. The tab stays open until the user
  // dismisses it.
  const markCommandExited = useCallback((id: string, code: number) => {
    setTabs((prev) =>
      prev.map((t) => (t.kind === "command" && t.id === id ? { ...t, exited: true, code } : t)),
    );
  }, []);

  // ctrl+q from the terminal layer. A finished command tab is closed (and the
  // workspace refreshed); anything still running just drops focus back to the
  // browser so the session / command keeps going.
  const termBack = useCallback(() => {
    const active = tabs.find((t) => tabKey(t) === activeTab);
    if (active && active.kind === "command" && active.exited) {
      closeTab(active.id);
      return;
    }
    setTermMode(false);
  }, [tabs, activeTab, closeTab]);

  if (!ls) {
    return (
      <box width="100%" height="100%" alignItems="center" justifyContent="center">
        <text fg={theme.textDim} attributes={DIM}>
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
          onRefresh={manualRefresh}
          onToggleProcs={() => setProcsOpen(true)}
          onQuit={quit}
          createBusy={createBusyLabel(createState)}
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
          onRefresh={manualRefresh}
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
        onBack={termBack}
        onSessionExit={closeTab}
        onCommandExit={markCommandExited}
      />

      {!connected ? (
        <box
          position="absolute"
          top={0}
          right={0}
          paddingTop={1}
          paddingRight={1}
          zIndex={100}
          flexDirection="row"
          justifyContent="flex-end"
        >
          <box
            border
            borderStyle="rounded"
            borderColor={theme.bad}
            backgroundColor={theme.surface}
            paddingLeft={1}
            paddingRight={1}
            flexDirection="row"
          >
            <text wrapMode="none">
              <span fg={theme.bad}>⚠</span>
              <span fg={theme.text}> sidecar disconnected — reconnecting…</span>
            </text>
          </box>
        </box>
      ) : null}

      <ToastLayer toasts={toasts} onDismiss={dismissToast} />

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

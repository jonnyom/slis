// Top-level app: owns the RPC client lifecycle, workspace data, live session
// badges, view routing (browser ⇄ cockpit) and the overlay layer (useOverlays).

import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react";
import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createRpcClient, isSliceNotFound } from "./rpc";
import type {
  AgentSpec,
  ConflictsResult,
  HelloResult,
  LsResult,
  PrStackEntry,
  RpcClient,
  SessionStatus,
  ShowResult,
  StatusEntry,
} from "./rpc/types";
import type { SliceView } from "./state/derive";
import { setTheme, theme, themeName, type ThemeName } from "./theme";
import { Browser } from "./views/browser";
import { Cockpit } from "./views/cockpit";
import type { CockpitEntry } from "./views/cockpit.hints";
import { AllSlicesProcOverlay } from "./components/procoverlay";
import { SessionOverlay } from "./components/sessionoverlay";
import { useOverlays, type OverlayApi } from "./overlays/useOverlays";
import { DIM } from "./components/ui";
import { TermManager } from "./term/manager";
import { TerminalLayer, tabKey, type TabEntry } from "./term/tabs";
import { resumeClaudeSession, tmuxAvailable, type TermMember } from "./term/tmux";
import type { OpenTermMode, TermSessionOpts } from "./term/session";
import { availableAgents, findSavedAgent, pickableAgents, agentCmdline } from "./term/agentpick";
import { availableEditors } from "./editor/detect";
import { bulkLoadPlan, loadSlicesSequentially, type BulkPhase } from "./state/bulkload";
import { BulkLoadOverlay } from "./components/bulkload";
import { useToasts, ToastLayer } from "./components/toast";
import { agentDefaultSet, createSlice } from "./rpc/mutate";
import {
  createBusyLabel,
  createReducer,
  initialCreateState,
  resolveCreatedSliceName,
} from "./state/create";
import { newlyDiscoveredSliceNames, shouldRefreshDiscovery, tickPlan } from "./state/tick";
import { isGatherableStackSlice } from "./state/cluster";
import { normalizeKeyName } from "./util/keys";
import {
  normalizeDiffScope,
  normalizeThemePreference,
  updatePrefs,
  type ThemePreference,
  type UiPrefs,
} from "./prefs";

export interface AppProps {
  initialPrefs: UiPrefs;
  initialThemeMode: "dark" | "light" | null;
}

const THEME_PREFERENCES: ThemePreference[] = ["auto", "midnight", "violet", "light"];

export function App({ initialPrefs, initialThemeMode }: AppProps): ReactNode {
  const renderer = useRenderer();
  const { width, height } = useTerminalDimensions();

  const clientRef = useRef<RpcClient | null>(null);
  if (!clientRef.current) clientRef.current = createRpcClient();
  const client = clientRef.current;

  const [hello, setHello] = useState<HelloResult | null>(null);
  const [ls, setLs] = useState<LsResult | null>(null);
  const lsRef = useRef<LsResult | null>(null);
  const [statuses, setStatuses] = useState<Record<string, SessionStatus>>({});
  const [statusEntries, setStatusEntries] = useState<StatusEntry[]>([]);
  const [prStacks, setPrStacks] = useState<Record<string, PrStackEntry[]>>({});
  const [shows, setShows] = useState<Record<string, ShowResult>>({});
  const [conflicts, setConflicts] = useState<ConflictsResult | null>(null);
  const [connected, setConnected] = useState(true);
  const [workspaceResyncNonce, setWorkspaceResyncNonce] = useState(0);

  const [view, setView] = useState<"browser" | "cockpit">("browser");
  const [current, setCurrent] = useState<string | null>(null);
  const currentRef = useRef<string | null>(null);
  currentRef.current = current;
  const [cockpitEntry, setCockpitEntry] = useState<CockpitEntry | null>(null);
  const [procsOpen, setProcsOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  const [activeTheme, setActiveTheme] = useState(themeName);
  const [uiPrefs, setUiPrefs] = useState(initialPrefs);
  const requestedTheme = process.env.SLIS_THEME?.trim().toLowerCase();
  const initialThemePreference: ThemePreference = requestedTheme
    ? requestedTheme === "auto" || requestedTheme === "system"
      ? "auto"
      : themeName()
    : normalizeThemePreference(initialPrefs.theme);
  const themePreferenceRef = useRef(initialThemePreference);
  const automaticThemeRef = useRef(initialThemePreference === "auto");
  const terminalThemeModeRef = useRef<"dark" | "light">(initialThemeMode ?? "dark");

  const persistUiPrefs = useCallback((patch: Partial<UiPrefs>) => {
    setUiPrefs((current) => ({ ...current, ...patch }));
    updatePrefs(patch);
  }, []);

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
  const bulkLoadRunRef = useRef(0);
  const [bulkPromptCount, setBulkPromptCount] = useState<number | null>(null);

  // Transient toasts (spec §3.5) + non-blocking create (spec D2).
  const { toasts, push: pushToast, dismiss: dismissToast } = useToasts();
  const [createState, dispatchCreate] = useReducer(createReducer, initialCreateState);
  const [browserFocusRequest, setBrowserFocusRequest] = useState<{
    id: number;
    slice: string;
  } | null>(null);
  const nextBrowserFocusRequest = useRef(0);

  // The slice the browser has focused (for the lazy-mode background tick, G7).
  const browserFocusRef = useRef<string | null>(null);

  const loadSlice = useCallback(
    (name: string): Promise<void> => {
      loadedRef.current.add(name);
      return Promise.allSettled([
        client.prStack(name).then((prs) => setPrStacks((m) => ({ ...m, [name]: prs }))),
        client.show(name).then((show) => setShows((m) => ({ ...m, [name]: show }))),
      ]).then((results) => {
        if (!results.some((result) => result.status === "rejected" && isSliceNotFound(result.reason))) {
          return;
        }
        setView("browser");
        setCurrent(null);
        setWorkspaceResyncNonce((nonce) => nonce + 1);
      });
    },
    [client],
  );

  const refresh = useCallback(
    (onDone?: (result: LsResult) => void) => {
      // ls first so the browser paints fast; the sidecar caps subprocess work at
      // 4 in flight, so the expensive fan-out (conflicts + per-slice PR/stack)
      // must wait behind ls rather than starve it.
      client.hello().then(setHello, () => {});
      client.status().then(
        (rows) => {
          setStatusEntries(rows);
          setStatuses(Object.fromEntries(rows.map((r) => [r.slice, r.status])));
        },
        () => {},
      );
      client.ls().then((res) => {
        lsRef.current = res;
        setLs(res);
        onDone?.(res);
        client.conflicts().then(setConflicts, () => {});
        const plan = bulkLoadPlan(res.slices.length, bulkPhaseRef.current);
        if (plan.prompt) {
          setBulkPromptCount(res.slices.length);
          return;
        }
        setBulkPromptCount(null);
        if (plan.fanOut) {
          loadedRef.current = new Set(res.slices.map((s) => s.name));
          const bulkLoadRun = ++bulkLoadRunRef.current;
          void loadSlicesSequentially(
            res.slices.map((s) => s.name),
            (name) =>
              bulkLoadRun === bulkLoadRunRef.current ? loadSlice(name) : Promise.resolve(),
          );
        }
      }, () => {});
    },
    [client, loadSlice],
  );

  const refreshDiscovery = useCallback(() => {
    return client.ls().then(
      (result) => {
        const previousNames = (lsRef.current?.slices ?? []).map((slice) => slice.name);
        const nextNames = result.slices.map((slice) => slice.name);
        const discovered = newlyDiscoveredSliceNames(previousNames, nextNames);
        const available = new Set(nextNames);
        lsRef.current = result;
        setLs(result);
        loadedRef.current = new Set(
          [...loadedRef.current].filter((name) => available.has(name)),
        );
        for (const name of discovered) void loadSlice(name);
        const active = currentRef.current;
        if (active && !available.has(active)) {
          setView("browser");
          setCurrent(null);
        }
      },
      () => {},
    );
  }, [client, loadSlice]);

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
        (rows) => {
          setStatusEntries(rows);
          setStatuses(Object.fromEntries(rows.map((r) => [r.slice, r.status])));
        },
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

  useEffect(() => {
    if (workspaceResyncNonce > 0) refresh();
  }, [workspaceResyncNonce, refresh]);

  const tickCtxRef = useRef({
    paused: false,
    focusedSlice: null as string | null,
  });
  tickCtxRef.current = {
    paused: termMode || bulkPromptCount !== null,
    focusedSlice: view === "cockpit" ? current : browserFocusRef.current,
  };
  useEffect(() => {
    const id = setInterval(() => {
      const plan = tickPlan(tickCtxRef.current);
      if (plan.run) tickRefresh(plan.slices);
      if (shouldRefreshDiscovery(tickCtxRef.current)) void refreshDiscovery();
    }, 30_000);
    return () => clearInterval(id);
  }, [refreshDiscovery, tickRefresh]);

  const quit = useCallback(() => {
    manager.detachAll(); // drop every tmux client — sessions keep running
    client.close();
    renderer.destroy();
    process.exit(0);
  }, [client, renderer, manager]);

  // Ctrl+C quits from every React UI state (browser, cockpit, diff, overlays),
  // before the key parser can turn it into the browser's plain `c` action.
  // A focused terminal is the exception: its raw-input handler must forward the
  // interrupt to the embedded shell/agent instead of exiting Slis.
  const termModeRef = useRef(termMode);
  termModeRef.current = termMode;
  useEffect(() => {
    const handler = (sequence: string): boolean => {
      if (sequence !== "\x03" || termModeRef.current) return false;
      quit();
      return true;
    };
    renderer.prependInputHandler(handler);
    return () => renderer.removeInputHandler(handler);
  }, [renderer, quit]);

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
            refresh((result) => {
              const createdSlice = resolveCreatedSliceName(result, name);
              if (!createdSlice) return;
              setBrowserFocusRequest({
                id: ++nextBrowserFocusRequest.current,
                slice: createdSlice,
              });
            });
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
    activeSlice: ls?.slices.find((slice) => slice.active)?.name,
    editors: editorList,
    configuredEditor: hello?.sessions.editor,
    runInteractive: openCommandTab,
    toast: pushToast,
    startCreate,
  });
  overlaysRef.current = overlays;

  // Theme switching is global across browser/cockpit/diff, but never steals a
  // T from a text-entry overlay or embedded terminal. Use the parsed key event
  // so Shift+T works under both legacy and modern kitty keyboard protocols.
  useKeyboard((key) => {
    const enabled = !overlays.active && !procsOpen && bulkPromptCount === null && !termMode;
    if (!enabled || normalizeKeyName(key) !== "T") return;
    const index = THEME_PREFERENCES.indexOf(themePreferenceRef.current);
    const next = THEME_PREFERENCES[(index + 1) % THEME_PREFERENCES.length]!;
    themePreferenceRef.current = next;
    automaticThemeRef.current = next === "auto";
    const applied = next === "auto"
      ? setTheme(terminalThemeModeRef.current === "light" ? "light" : "midnight")
      : setTheme(next as ThemeName);
    setActiveTheme(applied);
    persistUiPrefs({ theme: next });
    pushToast(`Theme: ${next === "auto" ? "system" : next}`, "idle");
  });

  // Some terminals report live profile/OS appearance changes. Continue
  // following those until the user explicitly cycles to a palette with T.
  useEffect(() => {
    const handler = (mode: "dark" | "light") => {
      terminalThemeModeRef.current = mode;
      if (!automaticThemeRef.current) return;
      const next = setTheme(mode === "light" ? "light" : "midnight");
      setActiveTheme(next);
    };
    renderer.on("theme_mode", handler);
    return () => {
      renderer.off("theme_mode", handler);
    };
  }, [renderer]);

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

  const agentList = useMemo(
    () => availableAgents(hello?.agents, (binary) => !!Bun.which(binary)),
    [hello?.agents],
  );
  // workspace.yaml is authoritative; the XDG preference is a migration
  // fallback for releases that saved the selection there only.
  const savedAgent = useMemo(
    () => findSavedAgent(agentList, hello?.sessions.default_agent, uiPrefs.agent),
    [agentList, hello?.sessions.default_agent, uiPrefs.agent],
  );
  const preferredAgent = savedAgent ?? agentList[0];

  const rememberAgent = useCallback(
    (choice: AgentSpec) => {
      persistUiPrefs({ agent: choice.name });
      agentDefaultSet(choice.name).then((result) => {
        if (result.code !== 0) {
          overlays.error(
            "Save default agent — failed",
            (result.stderr || result.stdout || "Unable to update workspace.yaml").trim(),
          );
        }
      });
    },
    [overlays, persistUiPrefs],
  );

  const configureAgents = useCallback(() => {
    const choices = agentList;
    if (choices.length === 0) {
      overlays.info(
        "No agents found",
        "Install a supported coding-agent CLI or add one under sessions.agents in workspace.yaml.",
      );
      return;
    }
    overlays.agentConfig(choices, preferredAgent?.name, (choice) => {
      rememberAgent(choice);
    });
  }, [agentList, overlays, preferredAgent, rememberAgent]);

  const onEnter = useCallback(
    (slice: string, entry?: CockpitEntry) => {
      if (bulkPhaseRef.current === "lazy" && !loadedRef.current.has(slice)) loadSlice(slice);
      setCockpitEntry(entry ?? null);
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
    (slice: string, mode: OpenTermMode, choice?: AgentSpec): TermSessionOpts | null => {
      const v = views.find((x) => x.slice.name === slice);
      if (!v) return null;
      const wsRoot = hello?.workspaceRoot ?? "";
      const sessions = hello?.sessions;
      const members: TermMember[] = v.slice.members.map((m) => ({
        repo: m.repo,
        branch: m.branch,
        worktreePath: m.worktree_path,
      }));
      const selectedAgent = choice ?? preferredAgent;
      const kind = mode === "shell" ? "shell" : "agent";
      return {
        slice,
        kind,
        members,
        active: v.slice.active,
        wsRoot,
        sessionOpts: { root: wsRoot, layout: sessions?.layout ?? "" },
        launchAgent: kind === "agent" && (mode === "agent-launch" || (sessions?.autostart ?? false)),
        agent: selectedAgent ? agentCmdline(selectedAgent.cmd) : sessions?.agent || "claude",
        harness: sessions?.harness || "claude",
        agentLabel: selectedAgent?.name,
      };
    },
    [views, hello, preferredAgent],
  );

  // Open (or reuse) the slice's terminal tab, attaching a tmux client and — when
  // launching an agent — running the picked (or default) agent in it.
  const launchTermTab = useCallback(
    (slice: string, mode: OpenTermMode, choice?: AgentSpec) => {
      const opts = buildTermOpts(slice, mode, choice);
      if (!opts) return;
      const key = `${opts.kind}:${slice}`;
      setTabs((prev) => {
        if (prev.some((t) => t.kind === "session" && t.slice === slice && t.opts.kind === opts.kind)) {
          return prev; // reuse the matching agent/shell tab; the other kind stays open
        }
        return [...prev, { kind: "session", slice, opts }];
      });
      setActiveTab(key);
      setTermMode(true);
    },
    [buildTermOpts],
  );

  const openTerm = useCallback(
    (slice: string, mode: OpenTermMode) => {
      if (!tmuxAvailable()) {
        overlays.info(
          "Terminal unavailable",
          "tmux is not on PATH — install tmux to use session tabs.",
        );
        return;
      }
      // With more than one configured agent, a launch (C / autostart) first asks
      // which one; a single agent (or an older sidecar) keeps the direct path.
      if (mode === "agent-launch") {
        // Once a valid default exists, C is a one-keystroke launch. The picker
        // is only the first-run choice; comma explicitly reconfigures it.
        if (savedAgent) {
          launchTermTab(slice, "agent-launch", savedAgent);
          return;
        }
        const choices = pickableAgents(agentList);
        if (choices.length > 1) {
          overlays.agentPicker(
            slice,
            choices,
            (choice) => {
              rememberAgent(choice);
              launchTermTab(slice, "agent-launch", choice);
            },
            preferredAgent?.name,
          );
          return;
        }
      }
      launchTermTab(slice, mode);
    },
    [launchTermTab, overlays, agentList, preferredAgent, rememberAgent, savedAgent],
  );

  const openExistingSession = useCallback(
    (slice: string | null, targetSession: string) => {
      const kind = targetSession.startsWith("slis-shell/") ? "shell" : "agent";
      const displaySlice = slice ?? targetSession.replace(/^slis(?:-shell)?\//, "");
      const opts: TermSessionOpts | null = slice
        ? buildTermOpts(slice, kind === "shell" ? "shell" : "agent")
        : {
            slice: displaySlice,
            kind,
            members: [],
            active: false,
            wsRoot: hello?.workspaceRoot ?? "",
            sessionOpts: {},
            launchAgent: false,
            agent: "",
            harness: hello?.sessions.harness || "claude",
          };
      if (!opts) return;
      const targetOpts = { ...opts, launchAgent: false, targetSession };
      const entry: TabEntry = { kind: "session", slice: displaySlice, opts: targetOpts };
      const key = tabKey(entry);
      setTabs((previous) =>
        previous.some((tab) => tabKey(tab) === key) ? previous : [...previous, entry],
      );
      setActiveTab(key);
      setTermMode(true);
    },
    [buildTermOpts, hello],
  );

  const resumeExistingClaudeSession = useCallback(
    (entry: StatusEntry) => {
      if (!entry.session_id) return;
      const opts = buildTermOpts(entry.slice, "agent");
      if (!opts) return;
      resumeClaudeSession({
        slice: entry.slice,
        sessionId: entry.session_id,
        cwd: entry.cwd,
        members: opts.members,
        sessionOpts: opts.sessionOpts,
      }).then(
        (target) => openExistingSession(entry.slice, target),
        (error) => overlays.info("Could not resume Claude", String(error)),
      );
    },
    [buildTermOpts, openExistingSession, overlays],
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
    void refreshDiscovery();
  }, [tabs, activeTab, closeTab, refreshDiscovery]);

  if (!ls) {
    return (
      <box
        width="100%"
        height="100%"
        alignItems="center"
        justifyContent="center"
        backgroundColor={theme.bg}
      >
        <text fg={theme.textDim} attributes={DIM}>
          {connected ? "loading workspace…" : "connecting to slis rpc…"}
        </text>
      </box>
    );
  }

  const bulkPromptOpen = bulkPromptCount !== null;
  const overlayEnabled = !overlays.active && !procsOpen && !sessionsOpen && !bulkPromptOpen;

  return (
    <box width="100%" height="100%" backgroundColor={theme.bg}>
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
          onConfigureAgents={configureAgents}
          onFocusSlice={onFocusSlice}
          initialFocusSlice={browserFocusRef.current}
          focusRequest={browserFocusRequest}
          onRefresh={manualRefresh}
          onToggleProcs={() => setProcsOpen(true)}
          onToggleSessions={() => setSessionsOpen(true)}
          onQuit={quit}
          createBusy={createBusyLabel(createState)}
          themeVersion={activeTheme}
        />
      ) : (
        <Cockpit
          enabled={overlayEnabled && !termMode}
          client={client}
          view={currentView}
          overlays={overlays}
          width={width}
          height={height}
          gatherable={isGatherableStackSlice(views, currentView.slice.name)}
          initialPanel={cockpitEntry?.panel}
          openCiLog={cockpitEntry?.ciLog}
          initialDiffMode={uiPrefs.split_diff ? "split" : "unified"}
          initialDiffScope={normalizeDiffScope(uiPrefs.diff_scope)}
          onDiffModeChange={(mode) => persistUiPrefs({ split_diff: mode === "split" })}
          onDiffScopeChange={(scope) => persistUiPrefs({ diff_scope: scope })}
          onBack={() => setView("browser")}
          onOpenTerm={openTerm}
          onOpenExistingSession={openExistingSession}
          onConfigureAgents={configureAgents}
          onToggleProcs={() => setProcsOpen(true)}
          onRefresh={manualRefresh}
          onRefreshSlice={loadSlice}
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
      {sessionsOpen ? (
        <SessionOverlay
          enabled={sessionsOpen && !termMode}
          views={views}
          statusEntries={statusEntries}
          onClose={() => setSessionsOpen(false)}
          onAttach={(slice, session) => {
            setSessionsOpen(false);
            openExistingSession(slice, session);
          }}
          onResume={(entry) => {
            setSessionsOpen(false);
            resumeExistingClaudeSession(entry);
          }}
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

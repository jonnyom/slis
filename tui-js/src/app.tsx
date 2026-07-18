// Top-level app: owns the RPC client lifecycle, workspace data, live session
// badges, view routing (browser ⇄ cockpit) and the help / swap overlays.

import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createRpcClient, usingFake } from "./rpc";
import type {
  ConflictsResult,
  HelloResult,
  LsResult,
  PrStackEntry,
  RpcClient,
  SessionStatus,
  ShowResult,
} from "./rpc/types";
import { activate, deactivate, type MutateResult } from "./rpc/mutate";
import type { SliceView } from "./state/derive";
import { color } from "./theme";
import { Browser } from "./views/browser";
import { Cockpit } from "./views/cockpit";
import { Help } from "./components/help";
import { Overlay } from "./components/overlay";
import { BOLD, DIM } from "./components/ui";
import { TermManager } from "./term/manager";
import { TerminalLayer, type TabEntry } from "./term/tabs";
import { tmuxAvailable, type TermMember } from "./term/tmux";
import type { TermSessionOpts } from "./term/session";

type Overlay =
  | { kind: "help" }
  | { kind: "swap"; slice: string; active: boolean }
  | { kind: "working"; text: string }
  | { kind: "result"; title: string; body: string; ok: boolean }
  | null;

function SwapOverlay({ slice, active }: { slice: string; active: boolean }): ReactNode {
  const verb = active ? "swap OUT" : "swap IN";
  const detail = active
    ? "Restores each primary to its previous branch."
    : "Puts each primary on slis/live/" + slice + " at the slice tip.";
  return (
    <Overlay title={`Swap — ${slice}`} width={56}>
      <text wrapMode="none">
        <span fg={color.fg}>{verb} </span>
        <span fg={color.title} attributes={BOLD}>
          {slice}
        </span>
        <span fg={color.fg}>?</span>
      </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        {detail}
      </text>
      <text> </text>
      <text wrapMode="none">
        <span fg={color.synced} attributes={BOLD}>
          [y]
        </span>
        <span fg={color.fg}> confirm   </span>
        <span fg={color.missing} attributes={BOLD}>
          [n/esc]
        </span>
        <span fg={color.fg}> cancel</span>
      </text>
    </Overlay>
  );
}

function ResultOverlay({
  title,
  body,
  ok,
}: {
  title: string;
  body: string;
  ok: boolean;
}): ReactNode {
  const lines = body.split("\n").slice(0, 14);
  return (
    <Overlay title={title} width={72}>
      {lines.map((l, i) => (
        <text key={i} fg={ok ? color.fg : color.missing} wrapMode="none">
          {l === "" ? " " : l}
        </text>
      ))}
      <text> </text>
      <text fg={color.dim} attributes={DIM}>
        press enter / esc to close
      </text>
    </Overlay>
  );
}

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
  const [overlay, setOverlay] = useState<Overlay>(null);

  // Embedded terminal session tabs.
  const managerRef = useRef<TermManager | null>(null);
  if (!managerRef.current) managerRef.current = new TermManager();
  const manager = managerRef.current;
  const [tabs, setTabs] = useState<TabEntry[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [termMode, setTermMode] = useState(false);

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
      for (const s of res.slices) {
        client.prStack(s.name).then(
          (prs) => setPrStacks((m) => ({ ...m, [s.name]: prs })),
          () => {},
        );
        client.show(s.name).then(
          (show) => setShows((m) => ({ ...m, [s.name]: show })),
          () => {},
        );
      }
    }, () => {});
  }, [client]);

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

  const runSwap = useCallback(
    async (slice: string, active: boolean) => {
      setOverlay({ kind: "working", text: `${active ? "Swapping out" : "Swapping in"} ${slice}…` });
      let res: MutateResult;
      try {
        res = active ? await deactivate(slice) : await activate(slice);
      } catch (err) {
        setOverlay({
          kind: "result",
          title: "Swap failed",
          body: String(err),
          ok: false,
        });
        return;
      }
      const ok = res.code === 0;
      setOverlay({
        kind: "result",
        title: ok ? "Swap done" : "Swap failed",
        body: (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
        ok,
      });
      refresh();
    },
    [refresh],
  );

  // Overlay keyboard (acts only while an overlay is open).
  useKeyboard((key) => {
    if (!overlay) return;
    const name = key.name;
    if (overlay.kind === "help") {
      if (name === "?" || name === "escape" || name === "q") setOverlay(null);
      return;
    }
    if (overlay.kind === "swap") {
      if (name === "y" || name === "return" || name === "enter")
        runSwap(overlay.slice, overlay.active);
      else if (name === "n" || name === "escape") setOverlay(null);
      return;
    }
    if (overlay.kind === "result") {
      if (name === "return" || name === "enter" || name === "escape" || name === "q")
        setOverlay(null);
      return;
    }
    // "working" overlays ignore input until they resolve.
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

  const onSwap = useCallback(
    (slice: string) => {
      const v = views.find((x) => x.slice.name === slice);
      setOverlay({ kind: "swap", slice, active: v?.slice.active ?? false });
    },
    [views],
  );

  const onEnter = useCallback((slice: string) => {
    setCurrent(slice);
    setView("cockpit");
  }, []);

  // ── embedded terminal tabs ────────────────────────────────────────────────

  // Build the attach options for a slice from live workspace data. Defaults to
  // the Go-TUI harness defaults (claude / root layout); slis rpc does not yet
  // expose the sessions config, so a custom agent/layout would need a new field.
  const buildTermOpts = useCallback(
    (slice: string, launchAgent: boolean): TermSessionOpts | null => {
      const v = views.find((x) => x.slice.name === slice);
      if (!v) return null;
      const wsRoot = hello?.workspaceRoot ?? "";
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
        sessionOpts: { root: wsRoot, layout: "" },
        launchAgent,
        agent: "claude",
        harness: "claude",
      };
    },
    [views, hello],
  );

  const openTerm = useCallback(
    (slice: string, launchAgent: boolean) => {
      if (!tmuxAvailable()) {
        setOverlay({
          kind: "result",
          title: "Terminal unavailable",
          body: "tmux is not on PATH — install tmux to use session tabs.",
          ok: false,
        });
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
    [buildTermOpts],
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

  const overlayEnabled = overlay === null;

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
          width={width}
          height={height}
          onEnter={onEnter}
          onSwap={onSwap}
          onOpenTerm={openTerm}
          onRefresh={refresh}
          onToggleHelp={() => setOverlay({ kind: "help" })}
          onQuit={quit}
        />
      ) : (
        <Cockpit
          enabled={overlayEnabled && !termMode}
          client={client}
          view={currentView}
          width={width}
          height={height}
          onBack={() => setView("browser")}
          onSwap={onSwap}
          onOpenTerm={openTerm}
          onToggleHelp={() => setOverlay({ kind: "help" })}
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
            {"  "}⚠ sidecar disconnected — reconnecting…{usingFake() ? "" : ""}
          </text>
        </box>
      ) : null}

      {overlay?.kind === "help" ? (
        <Help view={view === "cockpit" && currentView ? "cockpit" : "browser"} />
      ) : null}
      {overlay?.kind === "swap" ? (
        <SwapOverlay slice={overlay.slice} active={overlay.active} />
      ) : null}
      {overlay?.kind === "working" ? (
        <Overlay title="Working" width={48}>
          <text fg={color.fg}>{overlay.text}</text>
        </Overlay>
      ) : null}
      {overlay?.kind === "result" ? (
        <ResultOverlay title={overlay.title} body={overlay.body} ok={overlay.ok} />
      ) : null}
    </box>
  );
}

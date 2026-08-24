// The embedded-terminal layer: a tab bar plus one live ghostty terminal per
// open slice session. Terminals stay mounted while their tab is open, so the
// per-tab ghostty state persists and switching tabs is instant (no re-attach).
//
// Raw input: while the terminal is focused, every key except reserved Slis
// shortcuts is forwarded to the active PTY untouched.

import { extend, usePaste, useRenderer } from "@opentui/react";
import { useCallback, useEffect, useRef, type ReactNode } from "react";
import type { SessionStatus } from "../rpc/types";
import { color, sessionBadge, theme } from "../theme";
import { BOLD, DIM } from "../components/ui";
import { Divider } from "../components/divider";
import { TermManager } from "./manager";
import { tmuxWheelSequence } from "./mouse";
import type { TermSessionOpts } from "./session";
import { TERMINAL_FRAME_DELAY_MILLISECONDS, TerminalFeedBuffer } from "./feed";
import {
  embeddedTerminalInputSequence,
} from "./input";
import { requestTerminalFullRepaint } from "./repaint";
import { EmbeddedTerminalRenderable } from "./embedded";
import { useDelayedSessionAttachment } from "./attachment";

// Register <ghosttyTerminal> as an OpenTUI intrinsic element.
extend({ ghosttyTerminal: EmbeddedTerminalRenderable });
declare module "@opentui/react" {
  interface OpenTUIComponents {
    ghosttyTerminal: typeof EmbeddedTerminalRenderable;
  }
}

/** The reserved back key: ctrl+q (0x11). Overridable via SLIS_TERM_BACK_KEY. */
export const BACK_KEY = process.env["SLIS_TERM_BACK_KEY"]
  ? String.fromCharCode(parseInt(process.env["SLIS_TERM_BACK_KEY"]!, 16))
  : "\x11";
export const UNFOCUS_KEY = "\x07";
export const EMBEDDED_TERMINAL_SELECTABLE = true;
export const EMBEDDED_TERMINAL_BACKGROUND = theme.bg;
export const EMBEDDED_TERMINAL_HISTORY_LIMIT = 500;

export function isBackKeySequence(sequence: string): boolean {
  if (sequence === BACK_KEY) return true;
  const controlCode = BACK_KEY.charCodeAt(0);
  const keyCode = controlCode >= 1 && controlCode <= 26 ? controlCode + 96 : controlCode;
  return sequence === `\x1b[${keyCode};5u`;
}

export function isUnfocusKeySequence(sequence: string): boolean {
  return sequence === UNFOCUS_KEY || sequence === "\x1b[103;5u";
}

export function tabCycleDirection(sequence: string): -1 | 0 | 1 {
  if (sequence === "\x1b[57351;6u" || sequence === "\x1b[1;6C") return 1;
  if (sequence === "\x1b[57350;6u" || sequence === "\x1b[1;6D") return -1;
  return 0;
}

export function dockRefocusAction(
  name: string,
  ctrl: boolean,
  dockedTab: TabEntry | null | undefined,
): { activeTab: string; terminalFocused: true } | null {
  if (dockedTab?.kind !== "session" || !ctrl || name.toLowerCase() !== "g") return null;
  return { activeTab: tabKey(dockedTab), terminalFocused: true };
}

// A tmux-session tab (keyed by slice) or an interactive command tab (keyed by a
// unique id, running a one-shot mutation in a PTY). `exited` tracks a finished
// command so the tab bar / back key can offer to close it.
export type TabEntry =
  | { kind: "session"; slice: string; opts: TermSessionOpts }
  | { kind: "command"; id: string; title: string; argv: string[]; cwd?: string; exited: boolean; code?: number };

/** The stable id a tab is keyed by. Agent and shell tabs may coexist per slice. */
export function tabKey(t: TabEntry): string {
  return t.kind === "session"
    ? t.opts.targetSession
      ? `legacy:${t.opts.targetSession}`
      : `session:${t.slice}:${t.opts.tabID}`
    : t.id;
}

/** The label shown in the tab bar. Session tabs annotate the picked agent. */
export function tabLabel(t: TabEntry): string {
  if (t.kind !== "session") return t.title;
  if (t.opts.targetSession) return t.opts.targetSession.replace(/^slis(?:-shell)?\//, "");
  return `${t.slice} · ${t.opts.agentLabel ?? t.opts.tabTitle}`;
}

export function tabBarLabel(tab: TabEntry, tabs: TabEntry[]): string {
  if (tab.kind !== "session" || tab.opts.targetSession) return tabLabel(tab);
  const sessionTabs = tabs.filter(
    (entry): entry is Extract<TabEntry, { kind: "session" }> =>
      entry.kind === "session" && !entry.opts.targetSession,
  );
  if (sessionTabs.length > 0 && sessionTabs.every((entry) => entry.slice === tab.slice)) {
    return tab.opts.agentLabel ?? tab.opts.tabTitle;
  }
  return tabLabel(tab);
}

export function adjacentTabKey(tabs: TabEntry[], active: string | null, direction: -1 | 1): string | null {
  if (tabs.length === 0) return null;
  const current = tabs.findIndex((tab) => tabKey(tab) === active);
  const index = current < 0 ? 0 : (current + direction + tabs.length) % tabs.length;
  return tabKey(tabs[index]!);
}

export function sessionTabKeyForSlice(
  tabs: TabEntry[],
  slice: string,
  preferredKey?: string | null,
): string | null {
  const sessions = tabs.filter(
    (tab): tab is Extract<TabEntry, { kind: "session" }> =>
      tab.kind === "session" && tab.slice === slice,
  );
  const preferred = sessions.find((tab) => tabKey(tab) === preferredKey);
  const canonical = sessions.find((tab) => tab.opts.tabID === "agent");
  const selected = preferred ?? canonical ?? sessions[0];
  return selected ? tabKey(selected) : null;
}

export function nextSessionTabID(baseID: string, existingIDs: string[]): string {
  const existing = new Set(existingIDs);
  if (!existing.has(baseID)) return baseID;
  let suffix = 2;
  while (existing.has(`${baseID}-${suffix}`)) suffix += 1;
  return `${baseID}-${suffix}`;
}

export function terminalPresentation(
  appWidth: number,
  hasDockedSession: boolean,
  terminalFocused: boolean,
  activeTabIsSession: boolean,
  modalActive = false,
): {
  shown: boolean;
  docked: boolean;
  contentWidth: number;
  terminalLeft: number;
  terminalWidth: number;
} {
  if (modalActive) {
    return {
      shown: false,
      docked: false,
      contentWidth: appWidth,
      terminalLeft: 0,
      terminalWidth: appWidth,
    };
  }
  const canDock = appWidth >= 100;
  const docked = hasDockedSession && canDock && (!terminalFocused || activeTabIsSession);
  const shown = docked || terminalFocused;
  const terminalWidth = docked ? Math.max(48, Math.floor(appWidth * 0.42)) : appWidth;
  const contentWidth = docked ? appWidth - terminalWidth : appWidth;
  return {
    shown,
    docked,
    contentWidth,
    terminalLeft: docked ? contentWidth : 0,
    terminalWidth,
  };
}

// ── one terminal ─────────────────────────────────────────────────────────────

function TermTab({
  entry,
  manager,
  shown,
  focused,
  cols,
  rows,
  top,
  onFocus,
  onRenderable,
  onSessionExit,
  onCommandExit,
}: {
  entry: TabEntry;
  manager: TermManager;
  shown: boolean;
  focused: boolean;
  cols: number;
  rows: number;
  top: number;
  onFocus: () => void;
  onRenderable: (key: string, terminal: EmbeddedTerminalRenderable | null) => void;
  /** A tmux client died (session killed elsewhere) → close the tab. */
  onSessionExit: (key: string) => void;
  /** A command process exited → mark the tab exited (kept open for the user). */
  onCommandExit: (id: string, code: number) => void;
}): ReactNode {
  const renderer = useRenderer();
  const ref = useRef<EmbeddedTerminalRenderable>(null);
  const visibleRef = useRef(shown);
  visibleRef.current = shown;

  const key = tabKey(entry);
  const attachmentActive = useDelayedSessionAttachment(shown, entry.kind === "session");

  useEffect(() => {
    const terminal = ref.current;
    if (!terminal) return;
    onRenderable(key, terminal);
    return () => onRenderable(key, null);
  }, [key, onRenderable]);

  useEffect(() => {
    const terminal = ref.current;
    if (!terminal || !attachmentActive) return;
    const feedBuffer = new TerminalFeedBuffer(
      (bytes) => {
        terminal.feed(bytes);
        if (visibleRef.current) requestTerminalFullRepaint(renderer);
      },
      TERMINAL_FRAME_DELAY_MILLISECONDS,
      TERMINAL_FRAME_DELAY_MILLISECONDS,
    );
    const feed = (bytes: Uint8Array) => {
      feedBuffer.write(bytes);
    };
    if (entry.kind === "session") {
      const session = manager.session(key, entry.slice);
      const offExit = session.onExit(() => onSessionExit(key));
      let disposed = false;
      void session.attach(cols, rows, feed, entry.opts).then(
        () => {
          if (disposed) session.detach();
        },
        (err) => {
          if (disposed) return;
          terminal.feed(`\r\n[slis] failed to attach session: ${String(err)}\r\n`);
          renderer.requestRender();
        },
      );
      return () => {
        disposed = true;
        feedBuffer.cancel();
        offExit();
        manager.detach(key);
      };
    }
    const cmd = manager.command(key, entry.title, entry.argv, entry.cwd);
    const offExit = cmd.onExit((code) => {
      const ok = code === 0;
      terminal.feed(
        `\r\n[slis] ${entry.title} ${ok ? "finished" : `exited (code ${code})`}` +
          ` — press ctrl+q to close\r\n`,
      );
      renderer.requestRender();
      onCommandExit(key, code);
    });
    cmd.attach(cols, rows, feed).catch((err) => {
      terminal.feed(`\r\n[slis] failed to run ${entry.title}: ${String(err)}\r\n`);
      renderer.requestRender();
    });
    return () => {
      feedBuffer.cancel();
      offExit();
      manager.detach(key);
    };
  }, [attachmentActive]);

  // Propagate size changes to the PTY (the renderable's own cols/rows are set
  // via props on re-render).
  useEffect(() => {
    manager.get(key)?.resize(cols, rows);
  }, [manager, key, cols, rows]);

  // Cursor rendering is gated on focus; only the visible tab is focused.
  useEffect(() => {
    const terminal = ref.current;
    if (!terminal) return;
    if (focused) terminal.focus();
    else terminal.blur();
  }, [focused]);

  return (
    <ghosttyTerminal
      ref={ref}
      id={`term-pane-${key}`}
      position="absolute"
      left={0}
      top={top}
      width={cols}
      height={rows}
      cols={cols}
      rows={rows}
      bg={EMBEDDED_TERMINAL_BACKGROUND}
      limit={entry.kind === "session" ? EMBEDDED_TERMINAL_HISTORY_LIMIT : undefined}
      visible={shown}
      zIndex={101}
      persistent
      showCursor
      focusable
      selectable={EMBEDDED_TERMINAL_SELECTABLE}
      onMouseDown={() => {
        if (shown) onFocus();
      }}
      onMouseScroll={(event) => {
        if (!shown) return;
        if (entry.kind === "session" && manager.sessionApplicationHandlesMouse(key)) {
          const direction = event.scroll?.direction;
          if (!direction) return;
          const column = Math.min(cols, Math.max(1, event.x + 1));
          const row = Math.min(rows, Math.max(1, event.y - top + 1));
          manager.get(key)?.write(
            tmuxWheelSequence(direction, column, row, event.modifiers),
          );
          event.preventDefault();
        }
        event.stopPropagation();
      }}
    />
  );
}

// ── tab bar ──────────────────────────────────────────────────────────────────

export function TabBar({
  tabs,
  active,
  statuses,
  onBack,
  onUnfocus,
  onHide,
  onSelectTab,
  onCloseTab,
  onFocus,
}: {
  tabs: TabEntry[];
  active: string | null;
  statuses: Record<string, SessionStatus>;
  onBack: () => void;
  onUnfocus?: () => void;
  onHide?: () => void;
  onSelectTab: (key: string) => void;
  onCloseTab?: (key: string) => void;
  onFocus: () => void;
}): ReactNode {
  return (
    <box flexDirection="column" width="100%" height={2} zIndex={102} overflow="hidden">
      <box flexDirection="row" width="100%" height={1} backgroundColor={theme.surface} overflow="hidden">
        <box paddingLeft={1} paddingRight={1}>
          <text wrapMode="none" fg={theme.textFaint} attributes={BOLD}>TERM</text>
        </box>
        <text wrapMode="none" fg={theme.hairline}>│</text>
        {tabs.map((t) => {
          const key = tabKey(t);
          const on = key === active;
          const badge =
            t.kind === "session"
              ? t.opts.kind === "shell"
                ? { glyph: "›", color: color.live }
                : sessionBadge(statuses[t.slice] ?? "none")
              : {
                  glyph: t.exited ? (t.code === 0 ? "✓" : "✗") : "▸",
                  color: t.exited ? (t.code === 0 ? color.live : color.missing) : color.title,
                };
          return (
            <box
              key={key}
              height={1}
              backgroundColor={on ? theme.surfaceAlt : theme.surface}
              flexDirection="row"
            >
              <text
                id={`term-tab-${key}`}
                wrapMode="none"
                onMouseDown={(event) => {
                  onSelectTab(key);
                  onFocus();
                  event.preventDefault();
                  event.stopPropagation();
                }}
              >
                <span fg={on ? theme.focus : theme.hairline} attributes={BOLD}>{on ? "▎" : " "}</span>
                <span fg={badge.color} attributes={BOLD}>{` ${badge.glyph}`}</span>
                <span fg={on ? color.white : theme.textDim} attributes={on ? BOLD : 0}>
                  {` ${tabBarLabel(t, tabs)} `}
                </span>
              </text>
              {onCloseTab && t.kind === "session" && on ? (
                <text
                  id={`term-close-${key}`}
                  fg={theme.textDim}
                  attributes={BOLD}
                  wrapMode="none"
                  onMouseDown={(event) => {
                    onCloseTab(key);
                    event.preventDefault();
                    event.stopPropagation();
                  }}
                >
                  {"× "}
                </text>
              ) : null}
              <text wrapMode="none" fg={theme.hairline}>│</text>
            </box>
          );
        })}
        <box flexGrow={1} flexDirection="row" justifyContent="flex-end" paddingRight={1}>
          {tabs.length > 1 ? (
            <text fg={theme.textFaint} attributes={DIM} wrapMode="none">{"^⇧←/→ tabs  "}</text>
          ) : null}
          <text
            id={onUnfocus ? "term-unfocus" : "term-back"}
            fg={theme.textFaint}
            attributes={DIM}
            wrapMode="none"
            onMouseDown={(event) => {
              if (onUnfocus) onUnfocus();
              else onBack();
              event.preventDefault();
              event.stopPropagation();
            }}
          >
            {onUnfocus ? "ctrl+g slis" : "ctrl+q back"}
          </text>
          {onHide ? (
            <text
              id="term-hide"
              fg={theme.textDim}
              wrapMode="none"
              onMouseDown={(event) => {
                onHide();
                event.preventDefault();
                event.stopPropagation();
              }}
            >
              {"  ctrl+q hide ×"}
            </text>
          ) : null}
        </box>
      </box>
      <Divider />
    </box>
  );
}

// ── the layer ────────────────────────────────────────────────────────────────

export function TerminalLayer({
  tabs,
  tabBarTabs,
  active,
  shown,
  focused,
  statuses,
  width,
  height,
  left,
  manager,
  onBack,
  onUnfocus,
  onHide,
  onSelectTab,
  onCloseTab,
  onFocus,
  onSessionExit,
  onCommandExit,
}: {
  tabs: TabEntry[];
  tabBarTabs?: TabEntry[];
  active: string | null;
  shown: boolean;
  /** True when the terminal has raw input focus (overlaying the browser). */
  focused: boolean;
  statuses: Record<string, SessionStatus>;
  width: number;
  height: number;
  left: number;
  manager: TermManager;
  onBack: () => void;
  onUnfocus?: () => void;
  onHide?: () => void;
  onSelectTab: (key: string) => void;
  onCloseTab?: (key: string) => void;
  onFocus: () => void;
  onSessionExit: (key: string) => void;
  onCommandExit: (id: string, code: number) => void;
}): ReactNode {
  const renderer = useRenderer();
  const mountedTabKeysRef = useRef(new Set<string>());
  if (active) mountedTabKeysRef.current.add(active);

  // Keep the raw-input handler stable but reading live focus/active via refs.
  const focusedRef = useRef(focused);
  focusedRef.current = focused;
  const activeRef = useRef(active);
  activeRef.current = active;
  const cycleTabsRef = useRef(tabBarTabs ?? tabs);
  cycleTabsRef.current = tabBarTabs ?? tabs;
  const managerRef = useRef(manager);
  managerRef.current = manager;
  const terminalRenderablesRef = useRef(new Map<string, EmbeddedTerminalRenderable>());
  const registerTerminalRenderable = useCallback(
    (key: string, terminal: EmbeddedTerminalRenderable | null) => {
      if (terminal) terminalRenderablesRef.current.set(key, terminal);
      else terminalRenderablesRef.current.delete(key);
    },
    [],
  );
  const onBackRef = useRef(onBack);
  onBackRef.current = onBack;
  const onUnfocusRef = useRef(onUnfocus);
  onUnfocusRef.current = onUnfocus;
  const onSelectTabRef = useRef(onSelectTab);
  onSelectTabRef.current = onSelectTab;

  usePaste((event) => {
    if (!focusedRef.current) return;
    const key = activeRef.current;
    if (key) {
      terminalRenderablesRef.current.get(key)?.followLatestOutput();
      managerRef.current.get(key)?.paste(event.bytes);
    }
    event.preventDefault();
    event.stopPropagation();
  });

  useEffect(() => {
    const handler = (seq: string): boolean => {
      if (!focusedRef.current) return false; // browser/cockpit own the keys
      if (onUnfocusRef.current && isUnfocusKeySequence(seq)) {
        onUnfocusRef.current();
        return true;
      }
      const cycleDirection = tabCycleDirection(seq);
      if (cycleDirection !== 0) {
        const next = adjacentTabKey(cycleTabsRef.current, activeRef.current, cycleDirection);
        if (next) onSelectTabRef.current(next);
        return true;
      }
      if (isBackKeySequence(seq)) {
        onBackRef.current();
        return true;
      }
      const key = activeRef.current;
      if (key) {
        terminalRenderablesRef.current.get(key)?.followLatestOutput();
        managerRef.current.get(key)?.write(embeddedTerminalInputSequence(seq));
      }
      return true; // consume: everything reaches the PTY, nothing is parsed
    };
    renderer.addInputHandler(handler);
    return () => renderer.removeInputHandler(handler);
  }, [renderer]);

  if (tabs.length === 0) return null;

  const termRows = Math.max(2, height - 2);

  return (
    <box
      position="absolute"
      left={left}
      top={0}
      width={width}
      height={height}
      backgroundColor={theme.bg}
      visible={shown}
      zIndex={100}
    >
      <TabBar
        tabs={tabBarTabs ?? tabs}
        active={active}
        statuses={statuses}
        onBack={onBack}
        onUnfocus={onUnfocus}
        onHide={onHide}
        onSelectTab={onSelectTab}
        onCloseTab={onCloseTab}
        onFocus={onFocus}
      />
      {tabs.filter((tab) => mountedTabKeysRef.current.has(tabKey(tab))).map((t) => {
        const key = tabKey(t);
        return (
          <TermTab
            key={key}
            entry={t}
            manager={manager}
            shown={shown && key === active}
            focused={focused && key === active}
            cols={width}
            rows={termRows}
            top={2}
            onFocus={onFocus}
            onRenderable={registerTerminalRenderable}
            onSessionExit={onSessionExit}
            onCommandExit={onCommandExit}
          />
        );
      })}
    </box>
  );
}

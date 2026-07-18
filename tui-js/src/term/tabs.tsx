// The embedded-terminal layer: a tab bar plus one live ghostty terminal per
// open slice session. Terminals stay mounted while their tab is open, so the
// per-tab ghostty state persists and switching tabs is instant (no re-attach).
//
// Raw input: while the terminal is focused, every key except the reserved back
// key (ctrl+q) is forwarded to the active PTY untouched.

import { GhosttyTerminalRenderable } from "ghostty-opentui/terminal-buffer";
import { extend, useRenderer } from "@opentui/react";
import { useEffect, useRef, type ReactNode } from "react";
import type { SessionStatus } from "../rpc/types";
import { color, sessionBadge, theme } from "../theme";
import { BOLD, DIM } from "../components/ui";
import { TermManager } from "./manager";
import { tmuxWheelSequence } from "./mouse";
import type { TermSessionOpts } from "./session";

// Register <ghosttyTerminal> as an OpenTUI intrinsic element.
extend({ ghosttyTerminal: GhosttyTerminalRenderable });
declare module "@opentui/react" {
  interface OpenTUIComponents {
    ghosttyTerminal: typeof GhosttyTerminalRenderable;
  }
}

/** The reserved back key: ctrl+q (0x11). Overridable via SLIS_TERM_BACK_KEY. */
export const BACK_KEY = process.env["SLIS_TERM_BACK_KEY"]
  ? String.fromCharCode(parseInt(process.env["SLIS_TERM_BACK_KEY"]!, 16))
  : "\x11";

// A tmux-session tab (keyed by slice) or an interactive command tab (keyed by a
// unique id, running a one-shot mutation in a PTY). `exited` tracks a finished
// command so the tab bar / back key can offer to close it.
export type TabEntry =
  | { kind: "session"; slice: string; opts: TermSessionOpts }
  | { kind: "command"; id: string; title: string; argv: string[]; cwd?: string; exited: boolean; code?: number };

/** The stable id a tab is keyed by. Agent and shell tabs may coexist per slice. */
export function tabKey(t: TabEntry): string {
  return t.kind === "session" ? `${t.opts.kind}:${t.slice}` : t.id;
}

/** The label shown in the tab bar. Session tabs annotate the picked agent. */
export function tabLabel(t: TabEntry): string {
  if (t.kind !== "session") return t.title;
  if (t.opts.kind === "shell") return `${t.slice} · shell`;
  return t.opts.agentLabel ? `${t.slice} · ${t.opts.agentLabel}` : t.slice;
}

// ── one terminal ─────────────────────────────────────────────────────────────

function TermTab({
  entry,
  manager,
  visible,
  cols,
  rows,
  top,
  onSessionExit,
  onCommandExit,
}: {
  entry: TabEntry;
  manager: TermManager;
  visible: boolean;
  cols: number;
  rows: number;
  top: number;
  /** A tmux client died (session killed elsewhere) → close the tab. */
  onSessionExit: (key: string) => void;
  /** A command process exited → mark the tab exited (kept open for the user). */
  onCommandExit: (id: string, code: number) => void;
}): ReactNode {
  const renderer = useRenderer();
  const ref = useRef<GhosttyTerminalRenderable>(null);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;

  const key = tabKey(entry);

  // Attach the PTY once; detach on unmount (tab close / app quit). A tmux
  // session is never killed — detach only drops its client; a command PTY is
  // killed by detach only if still running.
  useEffect(() => {
    const term = ref.current;
    if (!term) return;
    const feed = (bytes: Uint8Array) => {
      term.feed(bytes);
      if (visibleRef.current) renderer.requestRender();
    };
    if (entry.kind === "session") {
      const session = manager.session(key, entry.slice);
      const offExit = session.onExit(() => onSessionExit(key));
      session.attach(cols, rows, feed, entry.opts).catch((err) => {
        term.feed(`\r\n[slis] failed to attach session: ${String(err)}\r\n`);
        renderer.requestRender();
      });
      return () => {
        offExit();
        manager.detach(key);
      };
    }
    const cmd = manager.command(key, entry.title, entry.argv, entry.cwd);
    const offExit = cmd.onExit((code) => {
      const ok = code === 0;
      term.feed(
        `\r\n[slis] ${entry.title} ${ok ? "finished" : `exited (code ${code})`}` +
          ` — press ctrl+q to close\r\n`,
      );
      renderer.requestRender();
      onCommandExit(key, code);
    });
    cmd.attach(cols, rows, feed).catch((err) => {
      term.feed(`\r\n[slis] failed to run ${entry.title}: ${String(err)}\r\n`);
      renderer.requestRender();
    });
    return () => {
      offExit();
      manager.detach(key);
    };
    // Attach once on mount; size/visibility are driven by the effects below.
  }, []);

  // Propagate size changes to the PTY (the renderable's own cols/rows are set
  // via props on re-render).
  useEffect(() => {
    manager.get(key)?.resize(cols, rows);
  }, [manager, key, cols, rows]);

  // Cursor rendering is gated on focus; only the visible tab is focused.
  useEffect(() => {
    const term = ref.current;
    if (!term) return;
    if (visible) term.focus();
    else term.blur();
  }, [visible]);

  return (
    <ghosttyTerminal
      ref={ref}
      position="absolute"
      left={0}
      top={top}
      width={cols}
      height={rows}
      cols={cols}
      rows={rows}
      visible={visible}
      zIndex={101}
      persistent
      showCursor
      focusable
      onMouseScroll={(event) => {
        if (!visible || entry.kind !== "session") return;
        const direction = event.scroll?.direction;
        if (!direction) return;

        // OpenTUI reports screen coordinates; tmux's SGR protocol expects
        // one-based coordinates relative to the embedded terminal.
        const column = Math.min(cols, Math.max(1, event.x + 1));
        const row = Math.min(rows, Math.max(1, event.y - top + 1));
        manager.get(key)?.write(
          tmuxWheelSequence(direction, column, row, event.modifiers),
        );
        event.preventDefault();
        event.stopPropagation();
      }}
    />
  );
}

// ── tab bar ──────────────────────────────────────────────────────────────────

function TabBar({
  tabs,
  active,
  statuses,
}: {
  tabs: TabEntry[];
  active: string | null;
  statuses: Record<string, SessionStatus>;
}): ReactNode {
  return (
    <box flexDirection="row" width="100%" height={1} zIndex={102}>
      <text wrapMode="none">
        <span fg={color.title} attributes={BOLD}>
          {" term "}
        </span>
        {tabs.map((t) => {
          const key = tabKey(t);
          const on = key === active;
          const glyph =
            t.kind === "session"
              ? t.opts.kind === "shell"
                ? { glyph: "›", color: color.live }
                : sessionBadge(statuses[t.slice] ?? "none")
              : {
                  glyph: t.exited ? (t.code === 0 ? "✓" : "✗") : "▸",
                  color: t.exited ? (t.code === 0 ? color.live : color.missing) : color.title,
                };
          return (
            <span key={key}>
              <span fg={color.dim}> </span>
              <span fg={glyph.color} attributes={BOLD}>
                {glyph.glyph}
              </span>
              <span fg={on ? color.white : color.fg} attributes={on ? BOLD : 0}>
                {" "}
                {tabLabel(t)}{" "}
              </span>
            </span>
          );
        })}
        <span fg={color.dim} attributes={DIM}>
          {"   ctrl+q back"}
        </span>
      </text>
    </box>
  );
}

// ── the layer ────────────────────────────────────────────────────────────────

export function TerminalLayer({
  tabs,
  active,
  focused,
  statuses,
  width,
  height,
  manager,
  onBack,
  onSessionExit,
  onCommandExit,
}: {
  tabs: TabEntry[];
  active: string | null;
  /** True when the terminal has raw input focus (overlaying the browser). */
  focused: boolean;
  statuses: Record<string, SessionStatus>;
  width: number;
  height: number;
  manager: TermManager;
  onBack: () => void;
  onSessionExit: (key: string) => void;
  onCommandExit: (id: string, code: number) => void;
}): ReactNode {
  const renderer = useRenderer();

  // Keep the raw-input handler stable but reading live focus/active via refs.
  const focusedRef = useRef(focused);
  focusedRef.current = focused;
  const activeRef = useRef(active);
  activeRef.current = active;
  const managerRef = useRef(manager);
  managerRef.current = manager;
  const onBackRef = useRef(onBack);
  onBackRef.current = onBack;

  useEffect(() => {
    const handler = (seq: string): boolean => {
      if (!focusedRef.current) return false; // browser/cockpit own the keys
      if (seq === BACK_KEY) {
        onBackRef.current();
        return true;
      }
      const key = activeRef.current;
      if (key) managerRef.current.get(key)?.write(seq);
      return true; // consume: everything reaches the PTY, nothing is parsed
    };
    renderer.addInputHandler(handler);
    return () => renderer.removeInputHandler(handler);
  }, [renderer]);

  if (tabs.length === 0) return null;

  const termRows = Math.max(2, height - 1); // row 0 is the tab bar

  return (
    <box
      position="absolute"
      left={0}
      top={0}
      width={width}
      height={height}
      backgroundColor={theme.bg}
      visible={focused}
      zIndex={100}
    >
      <TabBar tabs={tabs} active={active} statuses={statuses} />
      {tabs.map((t) => {
        const key = tabKey(t);
        return (
          <TermTab
            key={key}
            entry={t}
            manager={manager}
            visible={focused && key === active}
            cols={width}
            rows={termRows}
            top={1}
            onSessionExit={onSessionExit}
            onCommandExit={onCommandExit}
          />
        );
      })}
    </box>
  );
}

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
import { color, sessionBadge } from "../theme";
import { BOLD, DIM } from "../components/ui";
import { TermManager } from "./manager";
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

export interface TabEntry {
  slice: string;
  opts: TermSessionOpts;
}

// ── one terminal ─────────────────────────────────────────────────────────────

function TermTab({
  entry,
  manager,
  visible,
  cols,
  rows,
  top,
  onExit,
}: {
  entry: TabEntry;
  manager: TermManager;
  visible: boolean;
  cols: number;
  rows: number;
  top: number;
  onExit: (slice: string) => void;
}): ReactNode {
  const renderer = useRenderer();
  const ref = useRef<GhosttyTerminalRenderable>(null);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;

  const { slice } = entry;

  // Attach the PTY once; detach on unmount (tab close / app quit). The tmux
  // session is never killed — detach only drops this client.
  useEffect(() => {
    const term = ref.current;
    if (!term) return;
    const session = manager.session(slice);
    const offExit = session.onExit(() => onExit(slice));
    session
      .attach(
        cols,
        rows,
        (bytes) => {
          term.feed(bytes);
          if (visibleRef.current) renderer.requestRender();
        },
        entry.opts,
      )
      .catch((err) => {
        term.feed(`\r\n[slis] failed to attach session: ${String(err)}\r\n`);
        renderer.requestRender();
      });
    return () => {
      offExit();
      manager.detach(slice);
    };
    // Attach once on mount; size/visibility are driven by the effects below.
  }, []);

  // Propagate size changes to the PTY (the renderable's own cols/rows are set
  // via props on re-render).
  useEffect(() => {
    manager.session(slice).resize(cols, rows);
  }, [manager, slice, cols, rows]);

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
          const badge = sessionBadge(statuses[t.slice] ?? "none");
          const on = t.slice === active;
          return (
            <span key={t.slice}>
              <span fg={color.dim}> </span>
              <span fg={badge.color} attributes={BOLD}>
                {badge.glyph}
              </span>
              <span fg={on ? color.white : color.fg} attributes={on ? BOLD : 0}>
                {" "}
                {t.slice}{" "}
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
  onExit,
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
  onExit: (slice: string) => void;
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
      const slice = activeRef.current;
      if (slice) managerRef.current.session(slice).write(seq);
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
      visible={focused}
      zIndex={100}
    >
      <TabBar tabs={tabs} active={active} statuses={statuses} />
      {tabs.map((t) => (
        <TermTab
          key={t.slice}
          entry={t}
          manager={manager}
          visible={focused && t.slice === active}
          cols={width}
          rows={termRows}
          top={1}
          onExit={onExit}
        />
      ))}
    </box>
  );
}

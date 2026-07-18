// Transient confirmations (spec §3.5) — bottom-right stack, one short opacity
// fade in/out, auto-dismiss ~2.5s. For quick "swap done" / "copied" messages
// that don't deserve a modal. Timer-driven: one setTimeout per toast, cleared on
// unmount (spec §6). No polling, no per-frame reflow.

import { useCallback, useEffect, useReducer, useState, type ReactNode } from "react";
import { badgeFor, theme, type BadgeState } from "../theme";
import { BOLD } from "./ui";

export interface ToastItem {
  id: number;
  message: string;
  glyph: string;
  color: string;
}

export const TOAST_MAX = 4;

export type ToastAction =
  | { type: "push"; toast: ToastItem }
  | { type: "dismiss"; id: number };

// Pure queue reducer — capped at TOAST_MAX, oldest dropped first.
export function toastReducer(state: ToastItem[], action: ToastAction): ToastItem[] {
  switch (action.type) {
    case "push": {
      const next = [...state, action.toast];
      return next.length > TOAST_MAX ? next.slice(next.length - TOAST_MAX) : next;
    }
    case "dismiss":
      return state.filter((t) => t.id !== action.id);
    default:
      return state;
  }
}

let toastSeq = 0;

// Resolve a message + optional semantic state into a renderable toast (no id).
export function makeToast(
  message: string,
  state: BadgeState = "ci-pass",
): Omit<ToastItem, "id"> {
  const spec = badgeFor(state);
  return { message, glyph: spec.glyph, color: spec.color };
}

export interface Toasts {
  toasts: ToastItem[];
  push: (message: string, state?: BadgeState) => number;
  dismiss: (id: number) => void;
}

export function useToasts(): Toasts {
  const [toasts, dispatch] = useReducer(toastReducer, []);
  const dismiss = useCallback((id: number) => dispatch({ type: "dismiss", id }), []);
  const push = useCallback((message: string, state: BadgeState = "ci-pass") => {
    const id = ++toastSeq;
    dispatch({ type: "push", toast: { id, ...makeToast(message, state) } });
    return id;
  }, []);
  return { toasts, push, dismiss };
}

const FADE_MS = 180;

function Toast({
  toast,
  ttlMs,
  onDismiss,
}: {
  toast: ToastItem;
  ttlMs: number;
  onDismiss: (id: number) => void;
}): ReactNode {
  const [opacity, setOpacity] = useState(0);
  useEffect(() => {
    const fadeIn = setTimeout(() => setOpacity(1), 16);
    const fadeOut = setTimeout(() => setOpacity(0), Math.max(0, ttlMs - FADE_MS));
    const done = setTimeout(() => onDismiss(toast.id), ttlMs);
    return () => {
      clearTimeout(fadeIn);
      clearTimeout(fadeOut);
      clearTimeout(done);
    };
  }, [toast.id, ttlMs, onDismiss]);
  return (
    <box
      border
      borderStyle="rounded"
      borderColor={toast.color}
      backgroundColor={theme.surface}
      paddingLeft={1}
      paddingRight={1}
      marginTop={1}
      opacity={opacity}
      flexDirection="row"
    >
      <text wrapMode="none">
        <span fg={toast.color} attributes={BOLD}>
          {toast.glyph}
        </span>
        <span fg={theme.text}> {toast.message}</span>
      </text>
    </box>
  );
}

export function ToastLayer({
  toasts,
  onDismiss,
  ttlMs = 2500,
}: {
  toasts: ToastItem[];
  onDismiss: (id: number) => void;
  ttlMs?: number;
}): ReactNode {
  if (toasts.length === 0) return null;
  return (
    <box
      position="absolute"
      bottom={0}
      right={0}
      flexDirection="column"
      alignItems="flex-end"
      paddingRight={1}
      paddingBottom={1}
      zIndex={100}
    >
      {toasts.map((t) => (
        <Toast key={t.id} toast={t} ttlMs={ttlMs} onDismiss={onDismiss} />
      ))}
    </box>
  );
}

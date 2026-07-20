import type { LsResult } from "../rpc/types";

// Non-blocking slice creation (parity gap D2). Mirrors the Go TUI: `slis create`
// runs in the background with an ambient spinner in the header while the user
// keeps navigating; completion surfaces a toast (success) or a Result overlay
// (failure). This is the pure state for that flow — a tiny machine so the
// app.tsx wiring is testable and has one source of truth for the busy label.

export type CreateState = { status: "idle" } | { status: "creating"; name: string };

export type CreateAction = { type: "start"; name: string } | { type: "finish" };

export const initialCreateState: CreateState = { status: "idle" };

export function createReducer(state: CreateState, action: CreateAction): CreateState {
  switch (action.type) {
    case "start":
      return { status: "creating", name: action.name };
    case "finish":
      return { status: "idle" };
    default:
      return state;
  }
}

// The ambient header label while a create is in flight, or null when idle.
export function createBusyLabel(state: CreateState): string | null {
  return state.status === "creating" ? `creating ${state.name}…` : null;
}

export function resolveCreatedSliceName(result: LsResult, requestedBranch: string): string | null {
  const exactSlice = result.slices.find((slice) => slice.name === requestedBranch);
  if (exactSlice) return exactSlice.name;
  return (
    result.slices.find((slice) =>
      slice.members.some((member) => member.branch === requestedBranch),
    )?.name ?? null
  );
}

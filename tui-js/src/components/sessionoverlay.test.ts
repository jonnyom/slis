import { describe, expect, test } from "bun:test";
import type { SliceView } from "../state/derive";
import type { TmuxSessionInfo } from "../term/tmux";
import { buildSessionRows } from "./sessionoverlay";

const view: SliceView = {
  slice: {
    name: "checkout",
    base: "",
    active: false,
    stale: false,
    members: [
      {
        repo: "api",
        branch: "checkout",
        worktree_path: "/worktrees/checkout/api",
        tip_sha: "abc",
      },
    ],
  },
  status: "waiting-input",
};

describe("buildSessionRows", () => {
  test("offers resume when the related tmux session contains only shells", () => {
    const sessions: TmuxSessionInfo[] = [
      {
        name: "slis/checkout",
        kind: "agent",
        panes: [{ path: "/worktrees/checkout/api", command: "zsh" }],
      },
    ];
    const rows = buildSessionRows(sessions, [view], [
      {
        slice: "checkout",
        status: "waiting-input",
        session_id: "session-123",
        cwd: "/worktrees/checkout/api",
      },
    ]);
    expect(rows[0]?.recovery?.session_id).toBe("session-123");
  });

  test("does not offer resume over a running related agent", () => {
    const sessions: TmuxSessionInfo[] = [
      {
        name: "slis/checkout",
        kind: "agent",
        panes: [{ path: "/worktrees/checkout/api", command: "claude" }],
      },
    ];
    const rows = buildSessionRows(sessions, [view], [
      { slice: "checkout", status: "waiting-input", session_id: "session-123" },
    ]);
    expect(rows[0]?.recovery).toBeUndefined();
  });

  test("shows a recoverable session even when its tmux session is gone", () => {
    const rows = buildSessionRows([], [view], [
      {
        slice: "checkout",
        status: "done",
        session_id: "session-123",
        cwd: "/worktrees/checkout/api",
      },
    ]);
    expect(rows).toEqual([
      {
        slice: "checkout",
        recovery: {
          slice: "checkout",
          status: "done",
          session_id: "session-123",
          cwd: "/worktrees/checkout/api",
        },
      },
    ]);
  });
});

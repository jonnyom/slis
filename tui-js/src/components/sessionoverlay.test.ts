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

// A worktree can move between slices (regrouped, or re-registered under a new
// slice), but a tmux session's pane cwds are frozen at creation. Observed live:
// `slis/wage-proration` held a pane inside unpaid-leave's nory worktree, so
// attributing sessions by pane location alone labelled it "unpaid-leave" — and
// attaching or resuming from that row would have driven the wrong slice's Claude.
describe("buildSessionRows attribution", () => {
  const sliceView = (name: string, worktree: string): SliceView => ({
    slice: {
      name,
      base: "",
      active: false,
      stale: false,
      members: [{ repo: "nory", branch: "b", worktree_path: worktree, tip_sha: "abc" }],
    },
    status: "none",
  });

  test("attributes a session to the slice that owns its name, not the pane's slice", () => {
    const unpaid = sliceView("unpaid-leave", "/wt/unpaid-nory");
    const wage = sliceView("wage-proration", "/wt/wage-nory");
    const sessions: TmuxSessionInfo[] = [
      {
        name: "slis/wage-proration",
        kind: "agent",
        // Stale cwd: this dir now belongs to unpaid-leave.
        panes: [{ path: "/wt/unpaid-nory", command: "claude" }],
      },
    ];

    const rows = buildSessionRows(sessions, [unpaid, wage], []);

    expect(rows[0]?.slice).toBe("wage-proration");
  });

  test("still attributes an orphan session (renamed slice) by pane location", () => {
    const current = sliceView("current", "/wt/api");
    const sessions: TmuxSessionInfo[] = [
      { name: "slis/old-name", kind: "agent", panes: [{ path: "/wt/api", command: "claude" }] },
    ];

    expect(buildSessionRows(sessions, [current], [])[0]?.slice).toBe("current");
  });

  test("a recoverable session is not merged into another slice's tmux session", () => {
    const unpaid = sliceView("unpaid-leave", "/wt/unpaid-nory");
    const wage = sliceView("wage-proration", "/wt/wage-nory");
    const sessions: TmuxSessionInfo[] = [
      {
        name: "slis/wage-proration",
        kind: "agent",
        panes: [{ path: "/wt/unpaid-nory", command: "zsh" }],
      },
    ];

    const rows = buildSessionRows(sessions, [unpaid, wage], [
      { slice: "unpaid-leave", status: "waiting-input", session_id: "session-9" },
    ]);

    const wageRow = rows.find((row) => row.session?.name === "slis/wage-proration");
    expect(wageRow?.recovery).toBeUndefined();
    expect(rows.find((row) => row.recovery?.session_id === "session-9")?.slice).toBe("unpaid-leave");
  });
});

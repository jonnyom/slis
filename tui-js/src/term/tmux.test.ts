import { describe, expect, test } from "bun:test";
import {
  parseTmuxSessions,
  agentLaunchLine,
  liveForeignAgentInMembers,
  preferredRunningAgentSession,
  sessionHasPaneOutsideMembers,
  sessionName,
  sessionWindows,
  tmuxSessionClaimableBySlice,
  tmuxSessionOwnedBySlice,
  tmuxSessionRelatedToMembers,
  type TermMember,
  type TmuxSessionInfo,
} from "./tmux";

describe("sessionName", () => {
  test("keeps the existing agent namespace for compatibility", () => {
    expect(sessionName("feature.one")).toBe("slis/feature-one");
  });

  test("gives ad-hoc shells an independent tmux namespace", () => {
    expect(sessionName("feature.one", "shell")).toBe("slis-shell/feature-one");
  });
});

test("multi-repo sessions default to the shared slice root", () => {
  expect(sessionWindows(members, { root: "/workspace" })).toEqual([
    { name: "root", cwd: "/workspace/.slis/worktrees/test" },
  ]);
});

const members: TermMember[] = [
  {
    repo: "web",
    branch: "test",
    worktreePath: "/workspace/.slis/worktrees/test/web",
  },
  {
    repo: "api",
    branch: "test",
    worktreePath: "/workspace/.slis/worktrees/test/api",
  },
];

describe("sessionHasPaneOutsideMembers", () => {
  test("accepts member roots and their subdirectories", () => {
    expect(
      sessionHasPaneOutsideMembers(
        [
          "/workspace/.slis/worktrees/test/web",
          "/workspace/.slis/worktrees/test/api/app/services",
        ],
        members,
      ),
    ).toBe(false);
  });

  test("flags a legacy root pane in the shared parent", () => {
    expect(
      sessionHasPaneOutsideMembers(["/workspace/.slis/worktrees/test"], members),
    ).toBe(true);
  });

  test("flags the session when even one pane is outside the members", () => {
    expect(
      sessionHasPaneOutsideMembers(
        ["/workspace/.slis/worktrees/test/web", "/workspace"],
        members,
      ),
    ).toBe(true);
  });
});

describe("tmux session inventory", () => {
  const sessions = parseTmuxSessions(
    [
      "slis/old-name\t/workspace/.slis/worktrees/test/api\tclaude\tslis/old-name:0.0",
      "slis/old-name\t/workspace/.slis/worktrees/test/web\tzsh",
      "slis/current\t/workspace/.slis/worktrees/test/api\tzsh",
      "slis-shell/current\t/workspace/.slis/worktrees/test/web\tzsh",
      "unrelated\t/workspace\tzsh",
    ].join("\n"),
  );

  test("groups panes and excludes non-Slis sessions", () => {
    expect(sessions.map((session) => session.name)).toEqual([
      "slis-shell/current",
      "slis/current",
      "slis/old-name",
    ]);
    expect(sessions.find((session) => session.name === "slis/old-name")?.panes).toHaveLength(2);
    expect(sessions.find((session) => session.name === "slis/old-name")?.panes[0]?.target).toBe(
      "slis/old-name:0.0",
    );
  });

  test("relates legacy sessions by pane worktree path", () => {
    expect(
      tmuxSessionRelatedToMembers(
        sessions.find((session) => session.name === "slis/old-name")!,
        members,
      ),
    ).toBe(true);
  });

  test("prefers the related session with a running agent", () => {
    // "current" is the live slice; slis/old-name is its pre-rename orphan session.
    expect(preferredRunningAgentSession(sessions, members, "current", ["current"])?.name).toBe(
      "slis/old-name",
    );
  });
});

test("agent launch preserves Ghostty identity for notification clicks", () => {
  const previous = process.env.TERM_PROGRAM;
  process.env.TERM_PROGRAM = "ghostty";
  const line = agentLaunchLine({
    agent: "codex",
    harness: "codex",
    slice: "test",
    members,
    active: false,
    wsRoot: "/workspace",
  });
  if (previous === undefined) delete process.env.TERM_PROGRAM;
  else process.env.TERM_PROGRAM = previous;
  expect(line).toContain("SLIS_TERMINAL_APP='ghostty'");
});

test("agent launch starts from the shared slice root", () => {
  const line = agentLaunchLine({
    agent: "claude",
    harness: "claude",
    slice: "test",
    members,
    active: false,
    wsRoot: "/workspace",
  });
  expect(line).toStartWith("cd '/workspace/.slis/worktrees/test' && ");
});

// A tmux session's OWNER is its name (`slis/<slice>`). Pane cwds are frozen at
// session creation, so a worktree later regrouped into another slice leaves the
// original slice's session pointing into it. Observed live: `slis/wage-proration`
// held a pane in unpaid-leave's nory worktree, so the unpaid-leave cockpit both
// labelled it "‹this slice›" and would attach `a` to it.
describe("session ownership vs claimability", () => {
  const agentSession = (name: string, path: string, command = "claude"): TmuxSessionInfo => ({
    name,
    kind: "agent",
    panes: [{ target: name + ":0.0", path, command }],
  });
  const noryMembers: TermMember[] = [
    { repo: "nory", branch: "claude/x", worktreePath: "/wt/nory" },
  ];

  test("a slice owns the session named after it", () => {
    expect(tmuxSessionOwnedBySlice(agentSession("slis/unpaid-leave", "/wt/nory"), "unpaid-leave")).toBe(true);
  });

  test("ownership applies the same name sanitising as sessionName", () => {
    expect(tmuxSessionOwnedBySlice(agentSession("slis/feature-one", "/wt"), "feature.one")).toBe(true);
  });

  test("shell sessions are owned through their own namespace", () => {
    const shell: TmuxSessionInfo = {
      name: "slis-shell/unpaid-leave",
      kind: "shell",
      panes: [{ target: "slis-shell/unpaid-leave:0.0", path: "/wt/nory", command: "zsh" }],
    };
    expect(tmuxSessionOwnedBySlice(shell, "unpaid-leave")).toBe(true);
  });

  test("another live slice's session is NOT claimable, even with a pane in our worktree", () => {
    const foreign = agentSession("slis/wage-proration", "/wt/nory");
    // The pane-location heuristic alone says yes — that was the bug.
    expect(tmuxSessionRelatedToMembers(foreign, noryMembers)).toBe(true);
    expect(
      tmuxSessionClaimableBySlice(foreign, noryMembers, "unpaid-leave", [
        "unpaid-leave",
        "wage-proration",
      ]),
    ).toBe(false);
  });

  test("an orphan session (no slice owns its name) stays claimable by pane location", () => {
    const renamed = agentSession("slis/old-name", "/wt/nory");
    expect(
      tmuxSessionClaimableBySlice(renamed, noryMembers, "unpaid-leave", ["unpaid-leave"]),
    ).toBe(true);
  });
});

describe("preferredRunningAgentSession ownership", () => {
  const members: TermMember[] = [{ repo: "nory", branch: "claude/x", worktreePath: "/wt/nory" }];
  const agentSession = (name: string, command: string): TmuxSessionInfo => ({
    name,
    kind: "agent",
    panes: [{ target: name + ":0.0", path: "/wt/nory", command }],
  });

  test("never attaches to another live slice's agent", () => {
    expect(
      preferredRunningAgentSession([agentSession("slis/wage-proration", "claude")], members, "unpaid-leave", [
        "unpaid-leave",
        "wage-proration",
      ]),
    ).toBeUndefined();
  });

  test("still finds the slice's own running agent", () => {
    expect(
      preferredRunningAgentSession([agentSession("slis/unpaid-leave", "claude")], members, "unpaid-leave", [
        "unpaid-leave",
      ])?.name,
    ).toBe("slis/unpaid-leave");
  });

  test("ignores the slice's own session when it is only a shell prompt", () => {
    expect(
      preferredRunningAgentSession([agentSession("slis/unpaid-leave", "zsh")], members, "unpaid-leave", [
        "unpaid-leave",
      ]),
    ).toBeUndefined();
  });
});

// Two agents in one worktree can clobber each other's edits in a single git
// checkout. Before launching a new agent for a slice, slis looks for one already
// working in that slice's worktrees from a session the slice does not own.
describe("liveForeignAgentInMembers", () => {
  const members: TermMember[] = [
    { repo: "nory", branch: "b", worktreePath: "/wt/nory" },
    { repo: "web", branch: "b", worktreePath: "/wt/web" },
  ];
  const session = (name: string, path: string, command: string, kind: "agent" | "shell" = "agent"): TmuxSessionInfo => ({
    name,
    kind,
    panes: [{ target: name + ":0.0", path, command }],
  });

  test("finds another session's live agent inside our worktree", () => {
    const found = liveForeignAgentInMembers(
      [session("slis/wage-proration", "/wt/nory", "claude")],
      members,
      "unpaid-leave",
      ["unpaid-leave", "wage-proration"],
    );
    expect(found?.session.name).toBe("slis/wage-proration");
    expect(found?.pane.path).toBe("/wt/nory");
  });

  test("ignores our own session — the existing attach preference covers it", () => {
    expect(
      liveForeignAgentInMembers(
        [session("slis/unpaid-leave", "/wt/nory", "claude")],
        members,
        "unpaid-leave",
        ["unpaid-leave"],
      ),
    ).toBeUndefined();
  });

  test("ignores a foreign session that is only sitting at a shell", () => {
    expect(
      liveForeignAgentInMembers(
        [session("slis/wage-proration", "/wt/nory", "zsh")],
        members,
        "unpaid-leave",
        ["unpaid-leave", "wage-proration"],
      ),
    ).toBeUndefined();
  });

  test("ignores a foreign agent working outside our worktrees", () => {
    expect(
      liveForeignAgentInMembers(
        [session("slis/wage-proration", "/wt/elsewhere", "claude")],
        members,
        "unpaid-leave",
        ["unpaid-leave", "wage-proration"],
      ),
    ).toBeUndefined();
  });

  test("ignores shell-namespace sessions entirely", () => {
    expect(
      liveForeignAgentInMembers(
        [session("slis-shell/wage-proration", "/wt/nory", "claude", "shell")],
        members,
        "unpaid-leave",
        ["unpaid-leave", "wage-proration"],
      ),
    ).toBeUndefined();
  });
});

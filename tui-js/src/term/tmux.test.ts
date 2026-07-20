import { describe, expect, test } from "bun:test";
import {
  parseTmuxSessions,
  agentLaunchLine,
  preferredRunningAgentSession,
  sessionHasPaneOutsideMembers,
  sessionName,
  sessionWindows,
  tmuxSessionRelatedToMembers,
  type TermMember,
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
    expect(preferredRunningAgentSession(sessions, members)?.name).toBe("slis/old-name");
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

import { describe, expect, test } from "bun:test";
import { sessionHasPaneOutsideMembers, type TermMember } from "./tmux";

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

import { describe, expect, test } from "bun:test";
import {
  clipboardCandidates,
  mutationArgv,
  mutationRoute,
  spawnCapture,
  swapArgs,
  swapPlan,
} from "./mutate";

describe("mutationRoute", () => {
  test("submit/sync/merge/adopt/fix-ci route to a PTY (interactive)", () => {
    for (const c of ["submit", "sync", "merge", "adopt", "fix-ci"]) {
      expect(mutationRoute(c)).toBe("interactive");
    }
  });
  test("swap/lifecycle/grouping/ci-rerun stay captured", () => {
    for (const c of [
      "activate",
      "deactivate",
      "create",
      "rm",
      "restack",
      "group",
      "ungroup",
      "gather",
      "scatter",
      "import",
      "ignore",
      "editor",
      "agent",
      "ci-rerun",
      "edit",
      "summary",
      "pr-stack",
    ]) {
      expect(mutationRoute(c)).toBe("captured");
    }
  });
});

describe("swapArgs", () => {
  test("swap-in safely stashes dirty primaries", () => {
    expect(swapArgs("feature", false)).toEqual(["activate", "feature", "--stash"]);
  });

  test("swap-out deactivates the current journal", () => {
    expect(swapArgs("feature", true)).toEqual(["deactivate"]);
  });
});

describe("swapPlan", () => {
  test("hands off the live slice before activating another", () => {
    expect(swapPlan("test", false, "showing-mar")).toEqual([
      ["deactivate"],
      ["activate", "test", "--stash"],
    ]);
  });

  test("does not deactivate when no other slice is live", () => {
    expect(swapPlan("test", false)).toEqual([["activate", "test", "--stash"]]);
  });

  test("does not create a handoff from a stale self-reference", () => {
    expect(swapPlan("test", false, "test")).toEqual([["activate", "test", "--stash"]]);
  });

  test("swapping out remains a single deactivation", () => {
    expect(swapPlan("test", true, "test")).toEqual([["deactivate"]]);
  });
});

describe("mutationArgv", () => {
  test("prepends the slis binary and joins args", () => {
    expect(mutationArgv("submit", ["feat-x"])).toEqual(["slis", "submit", "feat-x"]);
  });
  test("defaults to no args", () => {
    expect(mutationArgv("sync")).toEqual(["slis", "sync"]);
  });
});

describe("spawnCapture timeout", () => {
  test("captures output and code when the command finishes in time", async () => {
    const res = await spawnCapture(["sh", "-c", "printf hi; printf oops 1>&2"], {
      timeoutMs: 5_000,
    });
    expect(res.code).toBe(0);
    expect(res.timedOut).toBeFalsy();
    expect(res.stdout).toBe("hi");
    expect(res.stderr).toBe("oops");
  });

  test("kills a child that exceeds its timeout and flags timedOut", async () => {
    const start = Date.now();
    const res = await spawnCapture(["sleep", "10"], { timeoutMs: 150 });
    const elapsed = Date.now() - start;
    expect(res.timedOut).toBe(true);
    expect(res.code).not.toBe(0);
    expect(res.stderr).toContain("timed out");
    expect(elapsed).toBeLessThan(3_000); // killed, not waited out
  });

  test("kills the whole process tree on timeout (no orphaned grandchild)", async () => {
    // The shell (a process-group leader via detached spawn) backgrounds a long
    // sleeper and prints its pid. A single-process kill would orphan the sleeper;
    // group-signalling the tree must reap it too.
    const res = await spawnCapture(["sh", "-c", "sleep 30 & echo $!; wait"], {
      timeoutMs: 200,
    });
    expect(res.timedOut).toBe(true);
    const grandchild = parseInt(res.stdout.trim(), 10);
    expect(Number.isInteger(grandchild)).toBe(true);
    // Give SIGTERM a beat to propagate through the group before checking.
    await new Promise((r) => setTimeout(r, 400));
    let alive = true;
    try {
      process.kill(grandchild, 0);
    } catch {
      alive = false; // ESRCH — the grandchild is gone
    }
    expect(alive).toBe(false);
  });
});

describe("clipboardCandidates", () => {
  test("darwin uses pbcopy", () => {
    expect(clipboardCandidates("darwin")).toEqual([{ cmd: "pbcopy", args: [] }]);
  });
  test("linux offers wl-copy then xclip then xsel", () => {
    expect(clipboardCandidates("linux").map((t) => t.cmd)).toEqual([
      "wl-copy",
      "xclip",
      "xsel",
    ]);
  });
  test("xclip / xsel carry their clipboard-selection args", () => {
    const linux = clipboardCandidates("linux");
    expect(linux.find((t) => t.cmd === "xclip")?.args).toEqual(["-selection", "clipboard"]);
    expect(linux.find((t) => t.cmd === "xsel")?.args).toEqual(["--clipboard", "--input"]);
  });
  test("unknown platform has no clipboard tool", () => {
    expect(clipboardCandidates("win32")).toEqual([]);
  });
});

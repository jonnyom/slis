import { expect, test, describe } from "bun:test";
import { availableAgents, findSavedAgent, pickableAgents, agentCmdline, quickPickIndex } from "./agentpick";
import type { AgentSpec } from "../rpc/types";

const CLAUDE: AgentSpec = { name: "claude", cmd: ["claude"] };
const CODEX: AgentSpec = { name: "codex", cmd: ["codex", "--full-auto"] };

describe("availableAgents", () => {
  test("keeps configured commands and detects installed common harnesses", () => {
    const found = new Set(["claude", "codex", "gemini", "opencode"]);
    expect(availableAgents([CODEX], (binary) => found.has(binary))).toEqual([
      CODEX,
      { name: "Claude Code", cmd: ["claude"] },
      { name: "Gemini CLI", cmd: ["gemini"] },
      { name: "OpenCode", cmd: ["opencode"] },
    ]);
  });

  test("does not invent missing agents or duplicate a configured binary", () => {
    expect(availableAgents([CLAUDE], (binary) => binary === "claude")).toEqual([CLAUDE]);
    const absolute = { name: "custom Claude", cmd: ["/opt/tools/claude", "--resume"] };
    expect(availableAgents([absolute], (binary) => binary === "claude")).toEqual([absolute]);
  });
});

describe("pickableAgents", () => {
  test("undefined (older sidecar) → no picker", () => {
    expect(pickableAgents(undefined)).toEqual([]);
  });
  test("empty list → no picker", () => {
    expect(pickableAgents([])).toEqual([]);
  });
  test("single agent → no picker", () => {
    expect(pickableAgents([CLAUDE])).toEqual([]);
  });
  test("two agents → picker", () => {
    expect(pickableAgents([CLAUDE, CODEX])).toEqual([CLAUDE, CODEX]);
  });
});

describe("findSavedAgent", () => {
  test("workspace default wins and launches without asking", () => {
    expect(findSavedAgent([CLAUDE, CODEX], "codex", "claude")).toBe(CODEX);
  });

  test("legacy preference migrates when workspace has no valid default", () => {
    expect(findSavedAgent([CLAUDE, CODEX], "removed-agent", "claude")).toBe(CLAUDE);
  });

  test("no valid saved choice returns to the picker", () => {
    expect(findSavedAgent([CLAUDE, CODEX], undefined, "removed-agent")).toBeUndefined();
  });
});

describe("agentCmdline", () => {
  test("bare binary stays unquoted (claude detection survives)", () => {
    expect(agentCmdline(["claude"])).toBe("claude");
  });
  test("flags join with spaces", () => {
    expect(agentCmdline(["codex", "--full-auto"])).toBe("codex --full-auto");
  });
  test("token with spaces gets single-quoted", () => {
    expect(agentCmdline(["aider", "--message", "hi there"])).toBe(
      "aider --message 'hi there'",
    );
  });
  test("embedded single quote is escaped", () => {
    expect(agentCmdline(["x", "a'b"])).toBe("x 'a'\\''b'");
  });
});

describe("quickPickIndex", () => {
  test("digit maps to zero-based index", () => {
    expect(quickPickIndex("1", 3)).toBe(0);
    expect(quickPickIndex("3", 3)).toBe(2);
  });
  test("out of range → null", () => {
    expect(quickPickIndex("4", 3)).toBeNull();
  });
  test("non-digit → null", () => {
    expect(quickPickIndex("j", 3)).toBeNull();
    expect(quickPickIndex("0", 3)).toBeNull();
  });
});

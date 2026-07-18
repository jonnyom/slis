import { expect, test, describe } from "bun:test";
import { pickableAgents, agentCmdline, quickPickIndex } from "./agentpick";
import type { AgentSpec } from "../rpc/types";

const CLAUDE: AgentSpec = { name: "claude", cmd: ["claude"] };
const CODEX: AgentSpec = { name: "codex", cmd: ["codex", "--full-auto"] };

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

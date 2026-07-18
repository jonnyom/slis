// Pure agent-picker logic, shared by the app (launch decision) and the overlay
// (quick-pick keys). The Go `hello` RPC resolves an `agents` list (name + argv);
// this module decides when a picker is warranted and turns a chosen argv into
// the shell command line the tmux launch path expects.

import type { AgentSpec } from "../rpc/types";

export const KNOWN_AGENTS: readonly AgentSpec[] = [
  { name: "Claude Code", cmd: ["claude"] },
  { name: "Codex", cmd: ["codex"] },
  { name: "Gemini CLI", cmd: ["gemini"] },
  { name: "Cursor Agent", cmd: ["cursor-agent"] },
  { name: "OpenCode", cmd: ["opencode"] },
];

// Preserve every configured command (including flags/custom wrappers), then
// append installed well-known harnesses that are not already represented.
export function availableAgents(
  configured: AgentSpec[] | undefined,
  onPath: (binary: string) => boolean,
): AgentSpec[] {
  const result = [...(configured ?? [])];
  const binaryName = (command: string | undefined) => command?.split(/[\\/]/).pop();
  const represented = new Set(result.map((agent) => binaryName(agent.cmd[0])).filter(Boolean));
  for (const agent of KNOWN_AGENTS) {
    const binary = agent.cmd[0]!;
    if (onPath(binary) && !represented.has(binary)) {
      result.push({ name: agent.name, cmd: [...agent.cmd] });
      represented.add(binary);
    }
  }
  return result;
}

// pickableAgents returns the agents worth showing a picker for — the list only
// when it holds more than one. Tolerates an older sidecar whose `hello` omitted
// the field (undefined → no picker, so the caller keeps the single-agent path).
export function pickableAgents(agents: AgentSpec[] | undefined): AgentSpec[] {
  const list = agents ?? [];
  return list.length > 1 ? list : [];
}

// Resolve a persisted default by display name. workspace.yaml wins, while the
// legacy XDG preference remains a fallback during migration. Stale names are
// ignored so a removed agent returns the user to the picker safely.
export function findSavedAgent(
  agents: AgentSpec[],
  configuredName?: string,
  legacyName?: string,
): AgentSpec | undefined {
  for (const name of [configuredName, legacyName]) {
    if (!name) continue;
    const found = agents.find((agent) => agent.name === name);
    if (found) return found;
  }
  return undefined;
}

const SHELL_SAFE = /^[A-Za-z0-9_@%+=:,./-]+$/;

// shellQuote single-quotes a token that contains shell-special characters,
// matching Go's shellSingleQuote (internal/tui/agentctx.go); plain tokens pass
// through so `["claude"]` stays "claude" and claude-agent detection still works.
function shellQuote(token: string): string {
  if (token === "") return "''";
  if (SHELL_SAFE.test(token)) return token;
  return "'" + token.replace(/'/g, "'\\''") + "'";
}

// agentCmdline joins an argv into one shell command line for the tmux send-keys
// launch path (the SLIS_* env prefix and claude's --append-system-prompt flag are
// added later by agentLaunchLine).
export function agentCmdline(cmd: string[]): string {
  return cmd.map(shellQuote).join(" ");
}

// quickPickIndex maps a "1".."9" digit key to a 0-based index within count, or
// null when the key isn't a digit or is out of range.
export function quickPickIndex(name: string, count: number): number | null {
  if (name.length !== 1 || name < "1" || name > "9") return null;
  const idx = name.charCodeAt(0) - "1".charCodeAt(0);
  return idx < count ? idx : null;
}

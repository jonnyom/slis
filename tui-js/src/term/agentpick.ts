// Pure agent-picker logic, shared by the app (launch decision) and the overlay
// (quick-pick keys). The Go `hello` RPC resolves an `agents` list (name + argv);
// this module decides when a picker is warranted and turns a chosen argv into
// the shell command line the tmux launch path expects.

import type { AgentSpec } from "../rpc/types";

// pickableAgents returns the agents worth showing a picker for — the list only
// when it holds more than one. Tolerates an older sidecar whose `hello` omitted
// the field (undefined → no picker, so the caller keeps the single-agent path).
export function pickableAgents(agents: AgentSpec[] | undefined): AgentSpec[] {
  const list = agents ?? [];
  return list.length > 1 ? list : [];
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

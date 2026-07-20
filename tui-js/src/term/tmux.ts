// tmux + agent-launch helpers, ported from the Go TUI so the JS terminal tabs
// create and populate a slice's session identically:
//   - session naming            → internal/tmuxctl.SessionName
//   - EnsureSession windows      → internal/tmuxctl.EnsureSession / sessionWindows
//   - SLIS_* env + slice context → internal/tui.agentLaunchLine (agentctx.go)
//
// We never own the agent process — we `tmux attach` a client (see session.ts).
// Everything here is a thin shell-out to the `tmux` binary; nothing mutates a repo.

import { dirname, isAbsolute, relative, resolve } from "node:path";

/** A slice member reduced to what session windows + agent context need. */
export interface TermMember {
  repo: string;
  branch: string;
  worktreePath: string;
}

/** Window-layout options, mirroring internal/tmuxctl.SessionOpts. */
export interface SessionOpts {
  /** Workspace root; enables a "root"/"both" window at the slice's shared parent. */
  root?: string;
  /** "root" | "repos" | "both". Empty → root when members share a parent. */
  layout?: string;
}

/** Agent and ad-hoc shell terminals intentionally use separate tmux sessions. */
export type SessionKind = "agent" | "shell";

export interface TmuxPane {
  path: string;
  command: string;
  target?: string;
}

export interface TmuxSessionInfo {
  name: string;
  kind: SessionKind;
  panes: TmuxPane[];
}

/** tmux disallows ':' and '.' in session names → replace with '-'. */
export function sessionName(slice: string, kind: SessionKind = "agent"): string {
  const prefix = kind === "agent" ? "slis/" : "slis-shell/";
  return prefix + slice.replace(/[:.]/g, "-");
}

async function sh(cmd: string[]): Promise<{ code: number; out: string }> {
  const p = Bun.spawn(cmd, { stdout: "pipe", stderr: "pipe", stdin: "ignore" });
  const out = await new Response(p.stdout).text();
  const code = await p.exited;
  return { code, out };
}

export function tmuxAvailable(): boolean {
  return Bun.which("tmux") !== null;
}

export function parseTmuxSessions(output: string): TmuxSessionInfo[] {
  const sessions = new Map<string, TmuxSessionInfo>();
  for (const line of output.split("\n")) {
    if (!line) continue;
    const [name, path, command, target] = line.split("\t");
    if (!name || !path || !command) continue;
    const kind = name.startsWith("slis/")
      ? "agent"
      : name.startsWith("slis-shell/")
        ? "shell"
        : null;
    if (!kind) continue;
    const session = sessions.get(name) ?? { name, kind, panes: [] };
    session.panes.push({ path, command, target });
    sessions.set(name, session);
  }
  return [...sessions.values()].sort((a, b) => a.name.localeCompare(b.name));
}

export async function listTmuxSessions(): Promise<TmuxSessionInfo[]> {
  const result = await sh([
    "tmux",
    "list-panes",
    "-a",
    "-F",
    "#{session_name}\t#{pane_current_path}\t#{pane_current_command}\t#{session_name}:#{window_index}.#{pane_index}",
  ]);
  return result.code === 0 ? parseTmuxSessions(result.out) : [];
}

export async function sessionExists(slice: string, kind: SessionKind = "agent"): Promise<boolean> {
  return (await sh(["tmux", "has-session", "-t", sessionName(slice, kind)])).code === 0;
}

/** Current directories for every pane in a slice session. */
export async function sessionPanePaths(slice: string, kind: SessionKind = "agent"): Promise<string[]> {
  const r = await sh([
    "tmux",
    "list-panes",
    "-s",
    "-t",
    sessionName(slice, kind),
    "-F",
    "#{pane_current_path}",
  ]);
  if (r.code !== 0) return [];
  return [...new Set(r.out.split("\n").map((path) => path.trim()).filter(Boolean))];
}

function pathIsWithin(path: string, parent: string): boolean {
  const rel = relative(resolve(parent), resolve(path));
  return rel === "" || (!rel.startsWith("..") && !isAbsolute(rel));
}

/** True when any session pane is operating outside all configured worktrees. */
export function sessionHasPaneOutsideMembers(paths: string[], members: TermMember[]): boolean {
  return paths.some((path) => !members.some((member) => pathIsWithin(path, member.worktreePath)));
}

export function tmuxSessionRelatedToMembers(
  session: TmuxSessionInfo,
  members: TermMember[],
): boolean {
  return session.panes.some((pane) =>
    members.some((member) => pathIsWithin(pane.path, member.worktreePath)),
  );
}

export function preferredRunningAgentSession(
  sessions: TmuxSessionInfo[],
  members: TermMember[],
): TmuxSessionInfo | undefined {
  return sessions.find(
    (session) =>
      session.kind === "agent" &&
      tmuxSessionRelatedToMembers(session, members) &&
      session.panes.some((pane) => !isShellCmd(pane.command)),
  );
}

export async function killTmuxSession(name: string): Promise<boolean> {
  if (!name.startsWith("slis/") && !name.startsWith("slis-shell/")) return false;
  return (await sh(["tmux", "kill-session", "-t", name])).code === 0;
}

export async function resumeClaudeSession(opts: {
  slice: string;
  sessionId: string;
  cwd?: string;
  members: TermMember[];
  sessionOpts: SessionOpts;
}): Promise<string> {
  if (!/^[A-Za-z0-9-]+$/.test(opts.sessionId)) throw new Error("invalid Claude session id");
  await ensureSession(opts.slice, opts.members, opts.sessionOpts, "agent");
  const name = sessionName(opts.slice);
  const session = (await listTmuxSessions()).find((candidate) => candidate.name === name);
	const target = session?.panes.find((pane) => isShellCmd(pane.command))?.target ?? name;
	const root = rootWindowCwd(opts.members);
	const resume = `claude --resume ${opts.sessionId}`;
	const command = root.ok ? `cd ${shellSingleQuote(root.cwd)} && ${resume}` : resume;
  const result = await sh([
    "tmux",
    "send-keys",
    "-t",
    target,
		command,
    "Enter",
  ]);
  if (result.code !== 0) throw new Error(`tmux send-keys: ${result.out}`);
  return name;
}

interface Window {
  name: string;
  cwd: string;
}

function perRepoWindows(sorted: TermMember[]): Window[] {
  return sorted.map((m) => ({ name: m.repo, cwd: m.worktreePath }));
}

// rootWindowCwd returns the directory a single "root" window cd's into so agents
// operate on the slice worktrees. For one member that is its worktree; for many
// it is their shared immediate parent. ok=false when they don't share one.
function rootWindowCwd(sorted: TermMember[]): { cwd: string; ok: boolean } {
  if (sorted.length === 0) return { cwd: "", ok: false };
  if (sorted.length === 1) return { cwd: sorted[0]!.worktreePath, ok: true };
  const parent = dirname(sorted[0]!.worktreePath);
  for (const m of sorted.slice(1)) {
    if (dirname(m.worktreePath) !== parent) return { cwd: "", ok: false };
  }
  return { cwd: parent, ok: true };
}

export function sessionWindows(members: TermMember[], opts: SessionOpts): Window[] {
  const sorted = [...members].sort((a, b) => a.repo.localeCompare(b.repo));

  let layout = opts.layout ?? "";
  if (layout === "") layout = opts.root ? "root" : "repos";

  let wins: Window[] = [];
  if ((layout === "root" || layout === "both") && opts.root) {
    const { cwd, ok } = rootWindowCwd(sorted);
    if (!ok) return perRepoWindows(sorted); // no shared parent → per-repo
    wins.push({ name: "root", cwd });
  }
  if (layout === "repos" || layout === "both") {
    wins = wins.concat(perRepoWindows(sorted));
  }
  if (wins.length === 0) return perRepoWindows(sorted);
  return wins;
}

// Claude exits on Ctrl-D (EOF); the correct, Claude-preserving way out is the
// tmux prefix detach (C-b d). Mirrors internal/tmuxctl.detachHint.
const DETACH_HINT = " detach: C-b d  (Ctrl-D quits Claude) ";

async function setStatusHint(name: string, kind: SessionKind): Promise<void> {
  // Per-session mouse mode lets the embedded client use wheel scrolling without
  // changing the user's global tmux configuration.
  await sh(["tmux", "set-option", "-t", name, "mouse", "on"]);
  await sh(["tmux", "set-option", "-t", name, "status-right-length", "40"]);
  const hint = kind === "agent" ? DETACH_HINT : " detach: C-b d  (ctrl+q returns to Slis) ";
  await sh(["tmux", "set-option", "-t", name, "status-right", hint]);
}

/**
 * Create the slice's tmux session (detached) if it does not already exist, with
 * windows determined by opts. Idempotent. Mirrors tmuxctl.EnsureSession.
 */
export async function ensureSession(
  slice: string,
  members: TermMember[],
  opts: SessionOpts,
  kind: SessionKind = "agent",
): Promise<void> {
  const name = sessionName(slice, kind);
  if (await sessionExists(slice, kind)) {
    await setStatusHint(name, kind);
    return;
  }

  const wins = sessionWindows(members, opts);
  if (wins.length === 0) {
    const r = await sh(["tmux", "new-session", "-d", "-s", name]);
    if (r.code !== 0) throw new Error(`tmux new-session: ${r.out}`);
    await setStatusHint(name, kind);
    return;
  }

  const first = wins[0]!;
  const args = ["new-session", "-d", "-s", name, "-n", first.name];
  if (first.cwd) args.push("-c", first.cwd);
  const created = await sh(["tmux", ...args]);
  if (created.code !== 0) throw new Error(`tmux new-session: ${created.out}`);

  for (const w of wins.slice(1)) {
    const a = ["new-window", "-t", name, "-n", w.name];
    if (w.cwd) a.push("-c", w.cwd);
    const r = await sh(["tmux", ...a]);
    if (r.code !== 0) throw new Error(`tmux new-window ${w.name}: ${r.out}`);
  }

  await setStatusHint(name, kind);
}

/** Foreground command of the session's active pane (e.g. "zsh", "claude"). */
export async function activePaneCommand(slice: string, kind: SessionKind = "agent"): Promise<string> {
  const r = await sh([
    "tmux",
    "display-message",
    "-p",
    "-t",
    sessionName(slice, kind),
    "#{pane_current_command}",
  ]);
  return r.code === 0 ? r.out.trim() : "";
}

/** Whether cmd is an interactive shell (safe to type a launch line into). */
export function isShellCmd(cmd: string): boolean {
  return ["zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh"].includes(cmd);
}

/** Type keys into the session's active pane followed by Enter. */
export async function sendKeys(slice: string, keys: string, kind: SessionKind = "agent"): Promise<void> {
  await sh(["tmux", "send-keys", "-t", sessionName(slice, kind), keys, "Enter"]);
}

// ── agent launch line (ported from internal/tui/agentctx.go) ─────────────────

function isClaudeAgent(agent: string): boolean {
  const bin = agent.trim().split(/\s+/)[0] ?? "";
  return bin === "claude" || bin.endsWith("/claude");
}

function shellSingleQuote(s: string): string {
  return "'" + s.replaceAll("'", `'\\''`) + "'";
}

function slisAgentContext(slice: string, members: TermMember[], active: boolean): string {
  const sorted = [...members].sort((a, b) => a.repo.localeCompare(b.repo));
  const parts = sorted.map((m) =>
    m.worktreePath
      ? `${m.repo} → ${m.worktreePath} (branch ${m.branch})`
      : `${m.repo} (branch ${m.branch})`,
  );
  let ctx =
    `You are running inside slis, a multi-repo worktree cockpit, working on slice "${slice}" ` +
    `which spans ${sorted.length} repo(s). Make ALL your edits inside this slice's git worktrees, listed here — ` +
    `do NOT touch the repos' primary checkouts: ${parts.join("; ")}. Each repo is a separate worktree on its own ` +
    `branch; cd into the right worktree for each repo and keep every commit scoped to that worktree.`;
  if (active) {
    ctx +=
      " (This slice is also LIVE — swapped into the primary checkouts so dev servers build it — " +
      "but still make every edit in the worktrees above, never the primaries.)";
  }
  return ctx;
}

function withSlisContext(agent: string, slice: string, members: TermMember[], active: boolean): string {
  if (!isClaudeAgent(agent)) return agent;
  return agent + " --append-system-prompt " + shellSingleQuote(slisAgentContext(slice, members, active));
}

function slisEnvPrefix(
  slice: string,
  members: TermMember[],
  active: boolean,
  wsRoot: string,
  harness: string,
): string {
  const sorted = [...members].sort((a, b) => a.repo.localeCompare(b.repo));
  const pairs = sorted.map((m) => `${m.repo}=${m.worktreePath}`);
  const vars = [
    "SLIS_SLICE=" + shellSingleQuote(slice),
    "SLIS_ROOT=" + shellSingleQuote(wsRoot),
    "SLIS_ACTIVE=" + shellSingleQuote(active ? "1" : "0"),
    "SLIS_HARNESS=" + shellSingleQuote(harness),
    "SLIS_WORKTREES=" + shellSingleQuote(pairs.join(",")),
  ];
  const terminalApp = process.env.SLIS_TERMINAL_APP ||
    (process.env.TERM_PROGRAM?.toLowerCase() === "ghostty" ? "ghostty" : "");
  if (terminalApp) vars.push("SLIS_TERMINAL_APP=" + shellSingleQuote(terminalApp));
  return vars.join(" ");
}

/**
 * The full one-line launch command: the SLIS_* env prefix followed by the agent
 * command (with claude's --append-system-prompt appended). Mirrors
 * internal/tui.agentLaunchLine.
 */
export function agentLaunchLine(opts: {
  agent: string;
  harness: string;
  slice: string;
  members: TermMember[];
  active: boolean;
  wsRoot: string;
}): string {
  const { agent, harness, slice, members, active, wsRoot } = opts;
	const launch = (
    slisEnvPrefix(slice, members, active, wsRoot, harness) +
    " " +
    withSlisContext(agent, slice, members, active)
  );
	const root = rootWindowCwd(members);
	return root.ok ? `cd ${shellSingleQuote(root.cwd)} && ${launch}` : launch;
}

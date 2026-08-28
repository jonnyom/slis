// tmux + agent-launch helpers, ported from the Go TUI so the JS terminal tabs
// create and populate a slice's session identically:
//   - session naming            → internal/tmuxctl.SessionName
//   - EnsureSession windows      → internal/tmuxctl.EnsureSession / sessionWindows
//   - SLIS_* env + slice context → internal/tui.agentLaunchLine (agentctx.go)
//
// We never own the agent process — we `tmux attach` a client (see session.ts).
// Everything here is a thin shell-out to the `tmux` binary; nothing mutates a repo.

import { dirname, isAbsolute, relative, resolve } from "node:path";
import {
  ensureSlisSession,
  ensureSlisTab,
  killLegacySession,
  killSlisSession,
  listLegacySessions,
  listSlisSessions,
  sendSlisInput,
  slisSessionBusy,
  type SessionGroup,
} from "./slis";

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

export function slisSessionAvailable(): boolean {
  return Bun.which(process.env["SLIS_BIN"] ?? "slis") !== null;
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

export function managedSessionTarget(groupID: string, tabID: string): string {
  return `session:${encodeURIComponent(groupID)}:${encodeURIComponent(tabID)}`;
}

export function parseManagedSessionTarget(target: string): { groupID: string; tabID: string } | undefined {
  const match = /^session:([^:]+):([^:]+)$/.exec(target);
  if (!match) return undefined;
  return {
    groupID: decodeURIComponent(match[1]!),
    tabID: decodeURIComponent(match[2]!),
  };
}

export function sessionDisplayName(target: string): string {
  const managed = parseManagedSessionTarget(target);
  if (!managed) return target.replace(/^slis(?:-shell)?\//, "");
  return `${managed.groupID} · ${managed.tabID.replace(/^repo:/, "")}`;
}

export function managedSessionInfos(groups: SessionGroup[]): TmuxSessionInfo[] {
  return groups.flatMap((group) => group.tabs.map((tab) => {
      const target = managedSessionTarget(group.id, tab.id);
      return {
        name: target,
        kind: tab.kind === "agent" || tab.kind === "review" ? "agent" : "shell",
        panes: [{ path: tab.cwd, command: tab.busy ? "slis-process" : "sh", target }],
      } satisfies TmuxSessionInfo;
    }));
}

export async function listTmuxSessions(): Promise<TmuxSessionInfo[]> {
  const groups = await listSlisSessions(true);
  const managed = managedSessionInfos(groups);
  const legacy = await listLegacySessions();
  return [...managed, ...legacy];
}

function pathIsWithin(path: string, parent: string): boolean {
  const rel = relative(resolve(parent), resolve(path));
  return rel === "" || (!rel.startsWith("..") && !isAbsolute(rel));
}

/** True when any session pane is operating outside all configured worktrees. */
export function sessionHasPaneOutsideMembers(paths: string[], members: TermMember[]): boolean {
  return paths.some((path) => !members.some((member) => pathIsWithin(path, member.worktreePath)));
}

/**
 * True when a session's PANES currently sit inside this slice's worktrees. This
 * is a proximity signal, NOT ownership: pane cwds are frozen when the session is
 * created, so a worktree later regrouped into another slice leaves the original
 * slice's session pointing into it. Use tmuxSessionOwnedBySlice to decide whose
 * session it is; use this only to explain a session's relationship to a slice.
 */
export function tmuxSessionRelatedToMembers(
  session: TmuxSessionInfo,
  members: TermMember[],
): boolean {
  return session.panes.some((pane) =>
    members.some((member) => pathIsWithin(pane.path, member.worktreePath)),
  );
}

/**
 * True when the session IS this slice's session — it carries the name this slice
 * would create (`sessionName`), which is the authoritative owner.
 */
export function tmuxSessionOwnedBySlice(session: TmuxSessionInfo, slice: string): boolean {
  const managed = parseManagedSessionTarget(session.name);
  if (managed) return managed.groupID === slice;
  return session.name === sessionName(slice, session.kind);
}

/** True when the session is named after some OTHER slice that still exists. */
export function tmuxSessionOwnedByAnotherSlice(
  session: TmuxSessionInfo,
  slice: string,
  knownSlices: string[],
): boolean {
  return knownSlices.some(
    (candidate) => candidate !== slice && tmuxSessionOwnedBySlice(session, candidate),
  );
}

/**
 * True when this slice may treat the session as its own — for labelling, and for
 * attaching to a running agent.
 *
 * Ownership by name comes first. A session named after no current slice is still
 * claimable by pane location: renaming a slice leaves its old session behind, and
 * that session holds the agent you were talking to.
 *
 * The case this exists to exclude: a session belonging to ANOTHER live slice whose
 * panes happen to sit in our worktrees. Pane cwds are frozen at session creation,
 * so a worktree regrouped into a different slice leaves the original slice's
 * session pointing into it — observed live, where `slis/wage-proration` held a
 * pane in unpaid-leave's nory worktree. Claiming it would label it as this
 * slice's and attach `a` to another slice's Claude.
 */
export function tmuxSessionClaimableBySlice(
  session: TmuxSessionInfo,
  members: TermMember[],
  slice: string,
  knownSlices: string[],
): boolean {
  if (tmuxSessionOwnedBySlice(session, slice)) return true;
  if (tmuxSessionOwnedByAnotherSlice(session, slice, knownSlices)) return false;
  return tmuxSessionRelatedToMembers(session, members);
}

/** A live agent pane found in a slice's worktrees, and the session running it. */
export interface LiveAgentPane {
  session: TmuxSessionInfo;
  pane: TmuxPane;
}

/**
 * The first live agent working inside this slice's worktrees from a session the
 * slice does NOT own. Two agents in one worktree share a single git checkout and
 * can overwrite each other's edits, so this is what slis checks before launching
 * another agent — the answer is "attach to that one", not "start a second".
 *
 * The slice's own sessions are skipped: preferredRunningAgentSession already
 * prefers reusing them. Panes sitting at a shell prompt are not agents.
 */
export function liveForeignAgentInMembers(
  sessions: TmuxSessionInfo[],
  members: TermMember[],
  slice: string,
  knownSlices: string[],
): LiveAgentPane | undefined {
  for (const session of sessions) {
    if (session.kind !== "agent") continue;
    if (tmuxSessionClaimableBySlice(session, members, slice, knownSlices)) continue;
    for (const pane of session.panes) {
      if (isShellCmd(pane.command)) continue;
      if (members.some((member) => pathIsWithin(pane.path, member.worktreePath))) {
        return { session, pane };
      }
    }
  }
  return undefined;
}

export function preferredRunningAgentSession(
  sessions: TmuxSessionInfo[],
  members: TermMember[],
  slice: string,
  knownSlices: string[],
): TmuxSessionInfo | undefined {
  return sessions.find(
    (session) =>
      session.kind === "agent" &&
      tmuxSessionClaimableBySlice(session, members, slice, knownSlices) &&
      session.panes.some((pane) => !isShellCmd(pane.command)),
  );
}

export async function killTmuxSession(name: string): Promise<boolean> {
  const managed = parseManagedSessionTarget(name);
  if (managed) return killSlisSession(managed.groupID);
  if (!name.startsWith("slis/") && !name.startsWith("slis-shell/")) return false;
  return killLegacySession(name);
}

export async function resumeClaudeSession(opts: {
  slice: string;
  sessionId: string;
  cwd?: string;
  members: TermMember[];
  sessionOpts: SessionOpts;
}): Promise<void> {
  if (!/^[A-Za-z0-9-]+$/.test(opts.sessionId)) throw new Error("invalid Claude session id");
  await ensureSlisSession(opts.slice);
  await ensureSlisTab(opts.slice, "agent", "agent");
  if (await slisSessionBusy(opts.slice, "agent")) throw new Error("agent terminal is busy");
	const root = rootWindowCwd(opts.members);
	const resume = `claude --resume ${opts.sessionId}`;
	const command = root.ok ? `cd ${shellSingleQuote(root.cwd)} && ${resume}` : resume;
  await sendSlisInput(opts.slice, "agent", command + "\r");
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

/** Whether cmd is an interactive shell (safe to type a launch line into). */
export function isShellCmd(cmd: string): boolean {
  return ["zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh"].includes(cmd);
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

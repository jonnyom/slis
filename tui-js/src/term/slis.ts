const BIN = process.env["SLIS_BIN"] ?? "slis";

export type SessionTabKind = "root" | "repo" | "agent" | "shell" | "review";

export interface SessionTab {
  id: string;
  kind: SessionTabKind;
  title: string;
  cwd: string;
  agent?: string;
  current_directory?: string;
  label?: string;
  busy?: boolean;
}

export interface SessionGroup {
  id: string;
  active_tab_id: string;
  tabs: SessionTab[];
}

export interface LegacyPane {
  path: string;
  command: string;
  target: string;
}

export interface LegacySession {
  name: string;
  kind: "agent" | "shell";
  panes: LegacyPane[];
}

export function sessionAttachArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "attach", groupID, tabID];
}

export function sessionActivateTabArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "activate-tab", groupID, tabID];
}

export function sessionSendArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "send", groupID, tabID];
}

export function sessionStartArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "start", groupID, tabID];
}

export function sessionEnsureTabArgv(
  groupID: string,
  tabID: string,
  kind: "agent" | "shell" | "review",
  title?: string,
): string[] {
  const argv = [BIN, "session", "ensure-tab", groupID, tabID, "--kind", kind];
  if (title) argv.push("--title", title);
  return argv;
}

export function sessionBusyArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "busy", groupID, tabID];
}

export function sessionLegacyAttachArgv(name: string): string[] {
  return [BIN, "session", "legacy-attach", name];
}

export function sessionLegacyListArgv(): string[] {
  return [BIN, "session", "legacy-list"];
}

export function sessionLegacyKillArgv(name: string): string[] {
  return [BIN, "session", "legacy-kill", name];
}

export function sessionLegacySendArgv(name: string): string[] {
  return [BIN, "session", "legacy-send", name];
}

export function sessionListArgv(live = false): string[] {
  return live ? [BIN, "session", "list", "--live"] : [BIN, "session", "list"];
}

export function sessionKillArgv(groupID: string): string[] {
  return [BIN, "session", "kill", groupID];
}

export function sessionKillTabArgv(groupID: string, tabID: string): string[] {
  return [BIN, "session", "kill-tab", groupID, tabID];
}

export async function ensureSlisSession(groupID: string): Promise<SessionGroup> {
  const result = await run([BIN, "session", "ensure", groupID]);
  return JSON.parse(result) as SessionGroup;
}

export async function activateSlisTab(groupID: string, tabID: string): Promise<void> {
  await run(sessionActivateTabArgv(groupID, tabID));
}

export async function ensureSlisTab(
  groupID: string,
  tabID: string,
  kind: "agent" | "shell" | "review",
  title?: string,
): Promise<SessionGroup> {
  const result = await run(sessionEnsureTabArgv(groupID, tabID, kind, title));
  return JSON.parse(result) as SessionGroup;
}

export async function slisSessionBusy(groupID: string, tabID: string): Promise<boolean> {
  const result = await run(sessionBusyArgv(groupID, tabID));
  return (JSON.parse(result) as { busy: boolean }).busy;
}

export async function listLegacySessions(): Promise<LegacySession[]> {
  return JSON.parse(await run(sessionLegacyListArgv())) as LegacySession[];
}

export async function listSlisSessions(live = false, signal?: AbortSignal): Promise<SessionGroup[]> {
  return JSON.parse(await run(sessionListArgv(live), signal)) as SessionGroup[];
}

export async function killLegacySession(name: string): Promise<boolean> {
  return (await runCode(sessionLegacyKillArgv(name))) === 0;
}

export async function killSlisSession(groupID: string): Promise<boolean> {
  return (await runCode(sessionKillArgv(groupID))) === 0;
}

export async function killSlisTab(groupID: string, tabID: string): Promise<boolean> {
  return (await runCode(sessionKillTabArgv(groupID, tabID))) === 0;
}

export async function sendLegacyInput(name: string, input: string): Promise<void> {
	await writeStdin(sessionLegacySendArgv(name), input, "legacy session input");
}

export async function sendSlisInput(groupID: string, tabID: string, input: Uint8Array | string): Promise<void> {
	await writeStdin(sessionSendArgv(groupID, tabID), input, "session input");
}

export async function startSlisCommand(groupID: string, tabID: string, command: string): Promise<void> {
	await writeStdin(sessionStartArgv(groupID, tabID), command, "session command");
}

async function writeStdin(argv: string[], input: Uint8Array | string, label: string): Promise<void> {
  const proc = Bun.spawn(argv, {
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  });
  proc.stdin.write(input);
  await proc.stdin.end();
  const [stderr, code] = await Promise.all([
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  if (code !== 0) throw new Error(stderr.trim() || `${label} exited with code ${code}`);
}

async function run(argv: string[], signal?: AbortSignal): Promise<string> {
  const proc = Bun.spawn(argv, { stdin: "ignore", stdout: "pipe", stderr: "pipe", signal });
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  if (code !== 0) throw new Error(stderr.trim() || `session command exited with code ${code}`);
  return stdout;
}

async function runCode(argv: string[]): Promise<number> {
  const proc = Bun.spawn(argv, { stdin: "ignore", stdout: "ignore", stderr: "ignore" });
  return proc.exited;
}

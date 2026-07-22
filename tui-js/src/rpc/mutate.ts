// Mutations are NOT part of the read-only sidecar. They run as one-shot
// `slis <cmd> …` spawns, exactly as the spike spec requires — the data-safety
// engine stays behind its tested CLI entry points. This module is the single
// shared runner: every mutation, clipboard write and URL open funnels through
// `spawnCapture`, so busy-state / error-surfacing has one code path.

import { fakeReviewAdd, fakeReviewRm, fakeReviewSend } from "./fakereviews";

const BIN = process.env["SLIS_BIN"] ?? "slis";

// Captured mutations get a generous wall-clock ceiling so a child that blocks on
// an unexpected prompt (stdin is ignored) can never hang the "working…" overlay
// forever — on expiry we kill the whole process tree and surface a clear error.
// Interactive mutations (submit/sync/merge/adopt/fix-ci) do NOT use this path;
// they run in a PTY tab where the user drives them (see mutationRoute).
export const DEFAULT_TIMEOUT_MS = 120_000;
// `create` spawns `git worktree add` across every repo and may fetch — give it
// far more headroom than the general default.
export const CREATE_TIMEOUT_MS = 600_000;

// Commands that must run in a real terminal: `gt submit/sync/merge` can prompt,
// `slis adopt` is an interactive branch picker, `slis fix-ci` launches `claude`.
export const INTERACTIVE_COMMANDS: ReadonlySet<string> = new Set([
  "submit",
  "sync",
  "merge",
  "adopt",
  "fix-ci",
]);

export type MutationRoute = "interactive" | "captured";

// Pure routing decision: which execution path a mutation command takes. Kept
// free of side effects so it is unit-testable and the single source of truth
// for both the overlay dispatcher and its tests.
export function mutationRoute(command: string): MutationRoute {
  return INTERACTIVE_COMMANDS.has(command) ? "interactive" : "captured";
}

function fake(): boolean {
  return process.env["SLIS_FAKE"] === "1";
}

// Exposed so the overlay layer can keep interactive commands on the captured
// path under SLIS_FAKE (no real PTY spawn in headless/test runs).
export function isFake(): boolean {
  return fake();
}

// The argv a mutation command runs as, for callers that spawn it themselves
// (e.g. the PTY tab for interactive commands). Mirrors `run` below.
export function mutationArgv(command: string, args: string[] = []): string[] {
  return [BIN, command, ...args];
}

export interface MutateResult {
  code: number;
  stdout: string;
  stderr: string;
  /** True when the child was killed after exceeding its timeout. */
  timedOut?: boolean;
}

interface SpawnCaptureOpts {
  stdinText?: string;
  timeoutMs?: number;
}

// Signal a whole process group (negative pid). `detached: true` makes the child
// a process-group leader (setsid), so signalling -pid reaches every descendant
// — no orphaned git/gh/gt grandchildren. Best-effort: once the tree exits the
// group is gone (ESRCH), which is the normal outcome and nothing to recover.
function signalTree(pid: number, signal: NodeJS.Signals): void {
  try {
    process.kill(-pid, signal);
  } catch {
    // Group already gone (ESRCH) — the tree is dead, which is the goal.
  }
}

// Exported for tests: the shared captured runner (timeout + process-tree kill).
export async function spawnCapture(cmd: string[], opts: SpawnCaptureOpts = {}): Promise<MutateResult> {
  const timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const proc = Bun.spawn({
    cmd,
    stdin: opts.stdinText !== undefined ? "pipe" : "ignore",
    stdout: "pipe",
    stderr: "pipe",
    // Own process group so a timeout can kill the whole tree, not just `slis`.
    detached: true,
  });
  if (opts.stdinText !== undefined && proc.stdin) {
    proc.stdin.write(opts.stdinText);
    await proc.stdin.end();
  }

  let timedOut = false;
  let escalation: ReturnType<typeof setTimeout> | null = null;
  const timer = setTimeout(() => {
    timedOut = true;
    signalTree(proc.pid, "SIGTERM");
    // A hung child that ignores SIGTERM is force-killed shortly after.
    escalation = setTimeout(() => signalTree(proc.pid, "SIGKILL"), 2_000);
  }, timeoutMs);

  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  clearTimeout(timer);
  if (escalation) clearTimeout(escalation);

  if (timedOut) {
    const secs = Math.round(timeoutMs / 1000);
    const note = `slis ${cmd.slice(1).join(" ")} timed out after ${secs}s — process tree killed`;
    const err = stderr.trim();
    return {
      code: code === 0 ? 124 : code,
      stdout: stdout.trim(),
      stderr: err ? `${err}\n${note}` : note,
      timedOut: true,
    };
  }
  return { code, stdout: stdout.trim(), stderr: stderr.trim() };
}

async function run(args: string[], timeoutMs?: number): Promise<MutateResult> {
  if (fake()) {
    return { code: 0, stdout: `(fake) would run: ${BIN} ${args.join(" ")}`, stderr: "" };
  }
  return spawnCapture([BIN, ...args], { timeoutMs });
}

// ── swap engine ──────────────────────────────────────────────────────────────

export function activate(slice: string): Promise<MutateResult> {
  return run(["activate", slice]);
}

// activate --stash: swap in AND stash any dirty work in the primaries first,
// popping it back on swap-out (the swap engine pins the stash by commit SHA).
export function activateStash(slice: string): Promise<MutateResult> {
  return run(["activate", slice, "--stash"]);
}

export function deactivate(): Promise<MutateResult> {
  return run(["deactivate"]);
}

// The interactive TUI chooses the recoverable --stash path for activation.
// Deactivation restores the pinned stash recorded by the Go swap engine.
export function swapArgs(slice: string, active: boolean): string[] {
  return active ? ["deactivate"] : ["activate", slice, "--stash"];
}

// A handoff is deliberately represented as two commands: deactivate must
// succeed before activate is attempted. Keeping the plan pure makes the safety
// invariant easy to test and keeps the confirmation UI honest.
export function swapPlan(slice: string, active: boolean, replacing?: string): string[][] {
  if (active) return [["deactivate"]];
  if (replacing && replacing !== slice) {
    return [["deactivate"], ["activate", slice, "--stash"]];
  }
  return [["activate", slice, "--stash"]];
}

export async function swapSlice(
  slice: string,
  active: boolean,
  replacing?: string,
): Promise<MutateResult> {
  const plan = swapPlan(slice, active, replacing);
  if (plan.length === 1) return run(plan[0]!);

  const deactivated = await run(plan[0]!);
  if (deactivated.code !== 0) return deactivated;

  const activated = await run(plan[1]!);
  if (activated.code === 0) return activated;

  const note = `${replacing} was swapped out, but ${slice} could not be swapped in. ` +
    `No slice is currently active; retry swapping in ${slice}.`;
  return {
    ...activated,
    stdout: [deactivated.stdout, activated.stdout].filter(Boolean).join("\n"),
    stderr: [note, activated.stderr].filter(Boolean).join("\n"),
  };
}

// ── slice lifecycle ──────────────────────────────────────────────────────────

export function createSlice(name: string): Promise<MutateResult> {
  return run(["create", name], CREATE_TIMEOUT_MS);
}

export function removeSlice(slice: string, force: boolean): Promise<MutateResult> {
  return run(force ? ["rm", slice, "--force"] : ["rm", slice]);
}

// ── graphite stack actions ───────────────────────────────────────────────────

export function restackSlice(slice: string): Promise<MutateResult> {
  return run(["restack", slice]);
}

export function submitSlice(slice: string): Promise<MutateResult> {
  return run(["submit", slice]);
}

export function mergeSlice(slice: string): Promise<MutateResult> {
  return run(["merge", slice]);
}

export function syncSlice(slice: string): Promise<MutateResult> {
  return run(["sync", slice]);
}

// ── CI (mutations — the sidecar stays read-only) ─────────────────────────────

// Re-trigger the failed CI runs for every repo's PR in a slice (`slis ci-rerun`,
// which wraps forge.RerunFailedChecks — the one CI write slis makes).
export function ciRerunSlice(slice: string): Promise<MutateResult> {
  return run(["ci-rerun", slice]);
}

// Point the agent harness at a slice's failing CI to fix it (`slis fix-ci`).
export function fixCiSlice(slice: string): Promise<MutateResult> {
  return run(["fix-ci", slice]);
}

// ── inline review (F2) — the sidecar stays read-only, so add/rm/send are
// one-shot `slis review …` spawns. Under SLIS_FAKE they drive the shared
// in-memory fake store instead, so the whole loop runs headlessly. ────────────

export interface ReviewAddInput {
  slice: string;
  repo: string;
  branch?: string;
  file: string;
  line: number;
  endLine?: number;
  side?: "new" | "old";
  hunk?: string;
  body: string;
}

export function reviewAdd(input: ReviewAddInput): Promise<MutateResult> {
  if (fake()) {
    const c = fakeReviewAdd(input);
    return Promise.resolve({
      code: 0,
      stdout: `added review comment ${c.id} on ${c.repo} ${c.file}:${c.line}`,
      stderr: "",
    });
  }
  const args = [
    "review",
    "add",
    input.slice,
    "--repo",
    input.repo,
    "--file",
    input.file,
    "--line",
    String(input.line),
    "--body",
    input.body,
  ];
  if (input.endLine && input.endLine > input.line)
    args.push("--end-line", String(input.endLine));
  if (input.side === "old") args.push("--side", "old");
  if (input.hunk) args.push("--hunk", input.hunk);
  return spawnCapture([BIN, ...args]);
}

export function reviewRm(slice: string, id: string): Promise<MutateResult> {
  if (fake()) {
    const ok = fakeReviewRm(slice, id);
    return Promise.resolve(
      ok
        ? { code: 0, stdout: `removed review comment ${id}`, stderr: "" }
        : { code: 1, stdout: "", stderr: `no pending review comment ${id} on slice ${slice}` },
    );
  }
  return spawnCapture([BIN, "review", "rm", slice, id]);
}

export function reviewSend(slice: string): Promise<MutateResult> {
  if (fake()) {
    const n = fakeReviewSend(slice);
    return Promise.resolve(
      n > 0
        ? { code: 0, stdout: `delivered ${n} review comment(s) to slice "${slice}"`, stderr: "" }
        : { code: 1, stdout: "", stderr: `no pending review comments for slice "${slice}"` },
    );
  }
  return spawnCapture([BIN, "review", "send", slice]);
}

export function reviewAgentArgs(slice: string, agent: string): string[] {
  return ["review", "agent", slice, "--agent", agent];
}

export function reviewAgent(slice: string, agent: string): Promise<MutateResult> {
  return run(reviewAgentArgs(slice, agent), 900_000);
}

// ── grouping ─────────────────────────────────────────────────────────────────

export function groupSlices(name: string, slices: string[]): Promise<MutateResult> {
  return run(["group", name, ...slices]);
}

export function ungroupSlice(name: string): Promise<MutateResult> {
  return run(["ungroup", name]);
}

// Gather the Graphite stack `slice` belongs to into one slice named `name`
// (represented by the stack tip; intermediates folded). Scatter reverses it.
export function gatherStack(name: string, slice: string): Promise<MutateResult> {
  return run(["gather", name, slice]);
}

export function scatterStack(name: string): Promise<MutateResult> {
  return run(["scatter", name]);
}

// ── candidate ingestion ──────────────────────────────────────────────────────

export function importCandidate(path: string): Promise<MutateResult> {
  return run(["import", path]);
}

export function ignoreCandidate(path: string): Promise<MutateResult> {
  return run(["ignore", path]);
}

export function adoptBranch(branch: string): Promise<MutateResult> {
  return run(["adopt", branch]);
}

// ── editor (one-shot spawns; `slis edit` launches detached, `slis editor set`
// persists the chosen editor to workspace.yaml exactly like the Go TUI) ───────

export function editorSet(bin: string): Promise<MutateResult> {
  return run(["editor", "set", bin]);
}

// Persist the selected launch agent in workspace.yaml. XDG UI preferences are
// still updated by the caller for compatibility with older slis releases.
export function agentDefaultSet(name: string): Promise<MutateResult> {
  return run(["agent", "set-default", name]);
}

export function editSlice(slice: string): Promise<MutateResult> {
  return run(["edit", slice]);
}

export function editRepo(slice: string, repo: string): Promise<MutateResult> {
  return run(["edit", slice, "--repo", repo]);
}

export function editPath(
  slice: string,
  repo: string,
  path: string,
  line?: number,
): Promise<MutateResult> {
  const args = ["edit", slice, "--repo", repo, "--file", path];
  if (line !== undefined && line > 0) args.push("--line", String(line));
  return run(args);
}

// ── summary (a read command, but a one-shot spawn like the mutations) ────────

export function summarySlice(slice: string, ai: boolean): Promise<MutateResult> {
  return run(ai ? ["summary", slice, "--ai"] : ["summary", slice]);
}

// pr-stack markdown on stdout (no --copy: we copy JS-side so the result pane can
// still show the markdown and the clipboard tool is chosen here, per the spec).
export function prStackMarkdown(slice: string): Promise<MutateResult> {
  return run(shareMarkdownArgs(slice));
}

export function shareMarkdownArgs(slice: string): string[] {
  return ["share", slice, "--stdout"];
}

// ── clipboard + open ─────────────────────────────────────────────────────────

export interface ClipboardTool {
  cmd: string;
  args: string[];
}

// Prioritised clipboard tools per platform — the same set the Go TUI probes
// (darwin: pbcopy; linux: wl-copy / xclip / xsel). Pure, so it is unit-testable.
export function clipboardCandidates(platform: NodeJS.Platform): ClipboardTool[] {
  if (platform === "darwin") return [{ cmd: "pbcopy", args: [] }];
  if (platform === "linux")
    return [
      { cmd: "wl-copy", args: [] },
      { cmd: "xclip", args: ["-selection", "clipboard"] },
      { cmd: "xsel", args: ["--clipboard", "--input"] },
    ];
  return [];
}

export async function copyToClipboard(text: string): Promise<MutateResult> {
  if (fake()) {
    return { code: 0, stdout: `(fake) copied ${text.length} chars`, stderr: "" };
  }
  for (const tool of clipboardCandidates(process.platform)) {
    if (Bun.which(tool.cmd)) {
      const res = await spawnCapture([tool.cmd, ...tool.args], { stdinText: text });
      if (res.code === 0) {
        return { code: 0, stdout: `copied to clipboard (${tool.cmd})`, stderr: "" };
      }
      return res;
    }
  }
  return {
    code: 1,
    stdout: "",
    stderr: "no clipboard tool found (need pbcopy / wl-copy / xclip / xsel)",
  };
}

export function openUrl(url: string): Promise<MutateResult> {
  if (fake()) return Promise.resolve({ code: 0, stdout: `(fake) would open ${url}`, stderr: "" });
  const opener = process.platform === "darwin" ? "open" : "xdg-open";
  return spawnCapture([opener, url]);
}

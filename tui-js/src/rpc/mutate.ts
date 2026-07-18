// Mutations are NOT part of the read-only sidecar. They run as one-shot
// `slis <cmd> …` spawns, exactly as the spike spec requires — the data-safety
// engine stays behind its tested CLI entry points. This module is the single
// shared runner: every mutation, clipboard write and URL open funnels through
// `spawnCapture`, so busy-state / error-surfacing has one code path.

const BIN = process.env["SLIS_BIN"] ?? "slis";

function fake(): boolean {
  return process.env["SLIS_FAKE"] === "1";
}

export interface MutateResult {
  code: number;
  stdout: string;
  stderr: string;
}

async function spawnCapture(cmd: string[], stdinText?: string): Promise<MutateResult> {
  const proc = Bun.spawn({
    cmd,
    stdin: stdinText !== undefined ? "pipe" : "ignore",
    stdout: "pipe",
    stderr: "pipe",
  });
  if (stdinText !== undefined && proc.stdin) {
    proc.stdin.write(stdinText);
    await proc.stdin.end();
  }
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  return { code, stdout: stdout.trim(), stderr: stderr.trim() };
}

async function run(args: string[]): Promise<MutateResult> {
  if (fake()) {
    return { code: 0, stdout: `(fake) would run: ${BIN} ${args.join(" ")}`, stderr: "" };
  }
  return spawnCapture([BIN, ...args]);
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

export function deactivate(slice: string): Promise<MutateResult> {
  return run(["deactivate", slice]);
}

// ── slice lifecycle ──────────────────────────────────────────────────────────

export function createSlice(name: string): Promise<MutateResult> {
  return run(["create", name]);
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

// ── grouping ─────────────────────────────────────────────────────────────────

export function groupSlices(name: string, slices: string[]): Promise<MutateResult> {
  return run(["group", name, ...slices]);
}

export function ungroupSlice(name: string): Promise<MutateResult> {
  return run(["ungroup", name]);
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

export function editSlice(slice: string): Promise<MutateResult> {
  return run(["edit", slice]);
}

export function editRepo(slice: string, repo: string): Promise<MutateResult> {
  return run(["edit", slice, "--repo", repo]);
}

// ── summary (a read command, but a one-shot spawn like the mutations) ────────

export function summarySlice(slice: string, ai: boolean): Promise<MutateResult> {
  return run(ai ? ["summary", slice, "--ai"] : ["summary", slice]);
}

// pr-stack markdown on stdout (no --copy: we copy JS-side so the result pane can
// still show the markdown and the clipboard tool is chosen here, per the spec).
export function prStackMarkdown(slice: string): Promise<MutateResult> {
  return run(["pr-stack", slice]);
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
      const res = await spawnCapture([tool.cmd, ...tool.args], text);
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

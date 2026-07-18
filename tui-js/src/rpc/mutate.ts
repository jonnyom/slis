// Mutations are NOT part of the read-only sidecar. They run as one-shot
// `slis <cmd> <slice>` spawns, exactly as the spike spec requires.

const BIN = process.env["SLIS_BIN"] ?? "slis";

export interface MutateResult {
  code: number;
  stdout: string;
  stderr: string;
}

async function run(args: string[]): Promise<MutateResult> {
  if (process.env["SLIS_FAKE"] === "1") {
    // No real repos under the fake client; report a clear no-op.
    return {
      code: 0,
      stdout: `(fake) would run: ${BIN} ${args.join(" ")}`,
      stderr: "",
    };
  }
  const proc = Bun.spawn({
    cmd: [BIN, ...args],
    stdin: "ignore",
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  return { code, stdout: stdout.trim(), stderr: stderr.trim() };
}

export function activate(slice: string): Promise<MutateResult> {
  return run(["activate", slice]);
}

export function deactivate(slice: string): Promise<MutateResult> {
  return run(["deactivate", slice]);
}

export interface LiveMirrorProcess {
  stdin: { end(): unknown };
  stderr: ReadableStream<Uint8Array>;
  exited: Promise<number>;
  kill(): void;
}

type SpawnLiveMirror = (command: string[]) => LiveMirrorProcess;

export function liveSyncCommand(binary = process.env["SLIS_BIN"] ?? "slis"): string[] {
  return [binary, "live-sync"];
}

function spawnLiveMirror(command: string[]): LiveMirrorProcess {
  const process = Bun.spawn({
    cmd: command,
    stdin: "pipe",
    stdout: "ignore",
    stderr: "pipe",
  });
  return process;
}

export class LiveMirrorManager {
  private process: LiveMirrorProcess | null = null;
  private stoppingProcess: LiveMirrorProcess | null = null;

  constructor(
    private readonly onFailure: (message: string) => void,
    private readonly spawn: SpawnLiveMirror = spawnLiveMirror,
    private readonly binary = process.env["SLIS_BIN"] ?? "slis",
  ) {}

  get running(): boolean {
    return this.process !== null;
  }

  start(): void {
    if (this.process) return;
    const process = this.spawn(liveSyncCommand(this.binary));
    this.process = process;
    const stderr = new Response(process.stderr).text();
    void Promise.all([process.exited, stderr]).then(([code, output]) => {
      if (this.process === process) this.process = null;
      if (this.stoppingProcess === process) {
        this.stoppingProcess = null;
        return;
      }
      const message = output.trim() || `Live sync stopped with exit code ${code}`;
      this.onFailure(message);
    });
  }

  async stop(): Promise<void> {
    const process = this.process;
    if (!process) return;
    this.stoppingProcess = process;
    this.process = null;
    await process.stdin.end();
    await process.exited;
    if (this.stoppingProcess === process) this.stoppingProcess = null;
  }
}

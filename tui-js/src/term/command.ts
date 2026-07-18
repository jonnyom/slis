// CommandSession: one interactive one-shot command running inside a Bun native
// PTY, surfaced as an embedded terminal tab. This is how slis runs the mutations
// that need a real TTY — `gt submit/sync/merge` (can prompt), `slis adopt` (a
// branch picker) and `slis fix-ci` (launches `claude`). It mirrors TermSession's
// PTY plumbing but, unlike a tmux client, it owns the child process: closing the
// tab (or app quit) kills it.
//
// The process is NOT reaped-and-forgotten on exit. When it exits we notify the
// exit handlers with the code so the tab can print an exit-status line and wait
// for the user to close it (ctrl+q), exactly like the parity spec requires.

interface PtyHandle {
  write(data: string | Uint8Array): void;
  resize(cols: number, rows: number): void;
  close?(): void;
}

export class CommandSession {
  readonly id: string;
  readonly title: string;
  private readonly argv: string[];
  private readonly cwd?: string;
  private readonly extraEnv?: Record<string, string>;
  private proc: { terminal: PtyHandle; kill(): void; exited: Promise<number> } | null = null;
  private started = false;
  private exited = false;
  private readonly exitHandlers = new Set<(code: number) => void>();

  constructor(id: string, title: string, argv: string[], cwd?: string, env?: Record<string, string>) {
    this.id = id;
    this.title = title;
    this.argv = argv;
    this.cwd = cwd;
    this.extraEnv = env;
  }

  get hasExited(): boolean {
    return this.exited;
  }

  /** Spawn the command in a fresh PTY. Idempotent: only the first call spawns. */
  async attach(cols: number, rows: number, onData: (bytes: Uint8Array) => void): Promise<void> {
    if (this.started) return;
    this.started = true;

    // Bun native PTY (Bun ≥ 1.3.5): the `terminal` option is not yet in
    // @types/bun, so the options object is typed loosely here (as TermSession).
    const spawnOpts = {
      cwd: this.cwd,
      env: { ...process.env, ...this.extraEnv, TERM: "xterm-256color" },
      terminal: {
        cols: Math.max(2, cols),
        rows: Math.max(2, rows),
        data: (_t: unknown, bytes: Uint8Array) => onData(bytes),
      },
    } as unknown as Parameters<typeof Bun.spawn>[1];
    const proc = Bun.spawn(this.argv, spawnOpts) as unknown as {
      terminal: PtyHandle;
      kill(): void;
      exited: Promise<number>;
    };

    this.proc = proc;
    proc.exited.then((code) => {
      this.exited = true;
      const c = typeof code === "number" ? code : 0;
      for (const h of this.exitHandlers) h(c);
    });
  }

  write(data: string | Uint8Array): void {
    if (this.exited) return; // no stdin once the command is done
    this.proc?.terminal.write(data);
  }

  resize(cols: number, rows: number): void {
    try {
      this.proc?.terminal.resize(Math.max(2, cols), Math.max(2, rows));
    } catch {
      // A resize racing a just-exited command is harmless; the PTY is gone.
    }
  }

  onExit(handler: (code: number) => void): () => void {
    this.exitHandlers.add(handler);
    return () => this.exitHandlers.delete(handler);
  }

  /** Close the PTY and kill the command if still running (tab close / app quit). */
  detach(): void {
    const proc = this.proc;
    this.proc = null;
    try {
      proc?.terminal.close?.();
    } catch {
      // Best-effort: closing an already-gone PTY is fine.
    }
    if (!this.exited) {
      try {
        proc?.kill();
      } catch {
        // Best-effort: killing an already-exited command is fine.
      }
    }
  }
}

// TermSession: one attached tmux client per slice, backed by a Bun native PTY.
//
// We spawn `tmux attach -t <session>` inside a Bun pseudo-terminal (Bun ≥ 1.3.5).
// The child is a tmux *client* — closing it (or killing this process) detaches;
// the tmux session (Claude/codex) keeps running. We NEVER kill the session here.
//
// The `data` callback delivers raw VT bytes; the caller feeds them to a ghostty
// renderable. `write` forwards raw keystrokes; `resize` propagates the pane size.

import { agentLaunchLine, activePaneCommand, ensureSession, isShellCmd, sessionName, sendKeys, type SessionOpts, type TermMember } from "./tmux";

export interface TermSessionOpts {
  slice: string;
  members: TermMember[];
  active: boolean;
  wsRoot: string;
  sessionOpts: SessionOpts;
  /** Launch the agent (SLIS_* env + claude context) when the pane is at a shell. */
  launchAgent: boolean;
  agent: string;
  harness: string;
  // Display name of the picked agent, shown in the tab label. Set only when the
  // agent picker chose one (>1 configured); undefined keeps the plain slice label.
  agentLabel?: string;
}

// Bun's PTY handle: .write / .resize / .close. Typed loosely — Bun.Terminal is
// not yet in @types/bun for this Bun version.
interface PtyHandle {
  write(data: string | Uint8Array): void;
  resize(cols: number, rows: number): void;
  close?(): void;
}

export class TermSession {
  readonly slice: string;
  private proc: { terminal: PtyHandle; kill(): void; exited: Promise<number> } | null = null;
  private detached = false;
  private readonly exitHandlers = new Set<() => void>();

  constructor(slice: string) {
    this.slice = slice;
  }

  get attached(): boolean {
    return this.proc !== null && !this.detached;
  }

  /**
   * Ensure the tmux session exists (creating windows the Go-TUI way), optionally
   * launch the agent, then attach a fresh PTY. Idempotent: a second call while
   * already attached is a no-op.
   */
  async attach(cols: number, rows: number, onData: (bytes: Uint8Array) => void, opts: TermSessionOpts): Promise<void> {
    if (this.attached) return;

    await ensureSession(opts.slice, opts.members, opts.sessionOpts);

    // Only type a launch line at a shell prompt — never into a running agent.
    if (opts.launchAgent && isShellCmd(await activePaneCommand(opts.slice))) {
      await sendKeys(
        opts.slice,
        agentLaunchLine({
          agent: opts.agent,
          harness: opts.harness,
          slice: opts.slice,
          members: opts.members,
          active: opts.active,
          wsRoot: opts.wsRoot,
        }),
      );
    }

    // Bun native PTY (Bun ≥ 1.3.5): the `terminal` option is not yet in
    // @types/bun, so the options object is typed loosely here.
    const spawnOpts = {
      env: { ...process.env, TERM: "xterm-256color" },
      terminal: {
        cols: Math.max(2, cols),
        rows: Math.max(2, rows),
        data: (_t: unknown, bytes: Uint8Array) => onData(bytes),
      },
    } as unknown as Parameters<typeof Bun.spawn>[1];
    const proc = Bun.spawn(["tmux", "attach", "-t", sessionName(opts.slice)], spawnOpts) as unknown as {
      terminal: PtyHandle;
      kill(): void;
      exited: Promise<number>;
    };

    this.proc = proc;
    this.detached = false;
    // If the attached client dies (e.g. session killed elsewhere), notify.
    proc.exited.then(() => {
      if (!this.detached) {
        this.detached = true;
        this.proc = null;
        for (const h of this.exitHandlers) h();
      }
    });
  }

  write(data: string | Uint8Array): void {
    this.proc?.terminal.write(data);
  }

  resize(cols: number, rows: number): void {
    try {
      this.proc?.terminal.resize(Math.max(2, cols), Math.max(2, rows));
    } catch {
      // A resize racing a just-exited client is harmless; the client is gone.
    }
  }

  onExit(handler: () => void): () => void {
    this.exitHandlers.add(handler);
    return () => this.exitHandlers.delete(handler);
  }

  /** Detach the client (close PTY + kill the attach process). Session survives. */
  detach(): void {
    if (this.detached) return;
    this.detached = true;
    const proc = this.proc;
    this.proc = null;
    try {
      proc?.terminal.close?.();
    } catch {
      // Best-effort: closing an already-gone PTY is fine.
    }
    try {
      proc?.kill();
    } catch {
      // Best-effort: killing an already-exited client is fine.
    }
  }
}

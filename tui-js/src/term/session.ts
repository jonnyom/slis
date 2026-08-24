import { agentLaunchLine, type SessionKind, type SessionOpts, type TermMember } from "./tmux";
import {
  ensureSlisSession,
  ensureSlisTab,
  startSlisCommand,
  sessionAttachArgv,
  sessionLegacyAttachArgv,
  slisSessionBusy,
} from "./slis";
import { TerminalModeTracker, TerminalQueryResponder } from "./responses";
import { embeddedTerminalPasteSequence, embeddedTerminalWriteSequence } from "./input";

/** User intent when opening a terminal from the TUI. */
export type OpenTermMode = "agent" | "agent-launch" | "agent-pick" | "shell";

export interface TermSessionOpts {
  slice: string;
  kind: SessionKind;
  tabID: string;
  tabTitle: string;
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
  targetSession?: string;
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
  private readonly terminalModes = new TerminalModeTracker();

  constructor(slice: string) {
    this.slice = slice;
  }

  get attached(): boolean {
    return this.proc !== null && !this.detached;
  }

  get applicationHandlesMouse(): boolean {
    return this.terminalModes.applicationHandlesMouse;
  }

  async attach(cols: number, rows: number, onData: (bytes: Uint8Array) => void, opts: TermSessionOpts): Promise<void> {
    if (this.attached) return;

    let attachArgv: string[];
    if (opts.targetSession) {
      attachArgv = sessionLegacyAttachArgv(opts.targetSession);
    } else {
      await ensureSlisSession(opts.slice);
      const tabID = opts.tabID;
      if (opts.kind === "agent" || opts.kind === "shell") {
        await ensureSlisTab(opts.slice, tabID, opts.kind);
      }
      if (opts.kind === "agent" && opts.launchAgent && !(await slisSessionBusy(opts.slice, tabID))) {
        const launch = agentLaunchLine({
          agent: opts.agent,
          harness: opts.harness,
          slice: opts.slice,
          members: opts.members,
          active: opts.active,
          wsRoot: opts.wsRoot,
        });
        await startSlisCommand(opts.slice, tabID, launch);
      }
      attachArgv = sessionAttachArgv(opts.slice, tabID);
    }

    // Bun native PTY (Bun ≥ 1.3.5): the `terminal` option is not yet in
    // @types/bun, so the options object is typed loosely here.
    const responder = new TerminalQueryResponder();
    const pendingResponses: string[] = [];
    let attachedProcess: { terminal: PtyHandle } | undefined;
    const spawnOpts = {
      env: { ...process.env, TERM: "xterm-256color" },
      terminal: {
        cols: Math.max(2, cols),
        rows: Math.max(2, rows),
        data: (_t: unknown, bytes: Uint8Array) => {
          this.terminalModes.observe(bytes);
          for (const response of responder.observe(bytes)) {
            if (attachedProcess) attachedProcess.terminal.write(response);
            else pendingResponses.push(response);
          }
          onData(bytes);
        },
      },
    } as unknown as Parameters<typeof Bun.spawn>[1];
    const proc = Bun.spawn(attachArgv, spawnOpts) as unknown as {
      terminal: PtyHandle;
      kill(): void;
      exited: Promise<number>;
    };

    this.proc = proc;
    attachedProcess = proc;
    for (const response of pendingResponses) proc.terminal.write(response);
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
    this.proc?.terminal.write(embeddedTerminalWriteSequence(data));
  }

  paste(bytes: Uint8Array): void {
    this.write(embeddedTerminalPasteSequence(bytes, this.terminalModes.bracketedPaste));
  }

  resize(cols: number, rows: number): void {
    this.proc?.terminal.resize(Math.max(2, cols), Math.max(2, rows));
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
    proc?.terminal.close?.();
    proc?.kill();
  }
}

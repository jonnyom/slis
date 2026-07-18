// Real RPC client: spawns the long-lived `slis rpc` Go sidecar and speaks
// JSON-RPC 2.0 over NDJSON (one JSON object per line) on its stdio. stderr is
// the sidecar's log stream and is inherited so it lands in our terminal's log.
//
// The sidecar is strictly read-only. Mutations (activate/deactivate) are
// one-shot `slis <cmd>` spawns from the UI, not part of this transport.

import type {
  CaptureResult,
  CommentsResult,
  ConflictsResult,
  DiffFormat,
  DiffResult,
  DiffScope,
  HelloResult,
  LsResult,
  ProcsResult,
  RpcClient,
  SessionEvent,
  ShowResult,
  StatusEntry,
  PrStackEntry,
} from "./types";

interface JsonRpcRequest {
  jsonrpc: "2.0";
  id: number;
  method: string;
  params?: unknown;
}

interface JsonRpcError {
  code: number;
  message: string;
  data?: { kind?: string } & Record<string, unknown>;
}

interface JsonRpcResponse {
  jsonrpc: "2.0";
  id: number;
  result?: unknown;
  error?: JsonRpcError;
}

interface JsonRpcNotification {
  jsonrpc: "2.0";
  method: string;
  params?: unknown;
}

/** An error raised by an RPC method, carrying the slis error `kind` when present. */
export class RpcError extends Error {
  readonly code: number;
  readonly kind?: string;
  constructor(err: JsonRpcError) {
    super(err.message);
    this.name = "RpcError";
    this.code = err.code;
    this.kind = err.data?.kind;
  }
}

interface Pending {
  resolve: (value: unknown) => void;
  reject: (reason: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
}

const REQUEST_TIMEOUT_MS = 30_000;
const BACKOFF_MIN_MS = 100;
const BACKOFF_MAX_MS = 5_000;

export interface SidecarOptions {
  /** Binary to run. Defaults to $SLIS_BIN or "slis". */
  bin?: string;
  /** Extra args after "rpc". */
  args?: string[];
}

export class SlisRpcClient implements RpcClient {
  private readonly bin: string;
  private readonly args: string[];

  private proc: Bun.Subprocess<"pipe", "pipe", "inherit"> | null = null;
  private nextId = 1;
  private readonly pending = new Map<number, Pending>();
  private readonly sessionHandlers = new Set<(e: SessionEvent) => void>();
  private readonly connectionHandlers = new Set<(connected: boolean) => void>();

  private buffer = "";
  private backoff = BACKOFF_MIN_MS;
  private restartTimer: ReturnType<typeof setTimeout> | null = null;
  private closed = false;

  constructor(opts: SidecarOptions = {}) {
    this.bin = opts.bin ?? process.env["SLIS_BIN"] ?? "slis";
    this.args = opts.args ?? [];
    this.spawn();
  }

  // ── lifecycle ─────────────────────────────────────────────────────────────

  private spawn(): void {
    if (this.closed) return;
    const proc = Bun.spawn({
      cmd: [this.bin, "rpc", ...this.args],
      stdin: "pipe",
      stdout: "pipe",
      stderr: "inherit",
    });
    this.proc = proc;
    this.buffer = "";
    this.readLoop(proc).catch((err) => this.onTransportDown(err));
    // When the process exits, tear down and schedule a restart.
    proc.exited
      .then((code) => this.onTransportDown(new Error(`sidecar exited (${code})`)))
      .catch((err) => this.onTransportDown(err));
    this.emitConnection(true);
  }

  private async readLoop(
    proc: Bun.Subprocess<"pipe", "pipe", "inherit">,
  ): Promise<void> {
    const decoder = new TextDecoder();
    const reader = proc.stdout.getReader();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      this.buffer += decoder.decode(value, { stream: true });
      let nl: number;
      while ((nl = this.buffer.indexOf("\n")) >= 0) {
        const line = this.buffer.slice(0, nl).trim();
        this.buffer = this.buffer.slice(nl + 1);
        if (line) this.handleLine(line);
      }
    }
  }

  private handleLine(line: string): void {
    let msg: JsonRpcResponse | JsonRpcNotification;
    try {
      msg = JSON.parse(line);
    } catch {
      // A malformed line is a sidecar bug; drop it rather than crash the UI.
      // stderr already carries the sidecar's own diagnostics.
      return;
    }
    if ("id" in msg && typeof msg.id === "number") {
      const pending = this.pending.get(msg.id);
      if (!pending) return;
      this.pending.delete(msg.id);
      clearTimeout(pending.timer);
      const res = msg as JsonRpcResponse;
      if (res.error) pending.reject(new RpcError(res.error));
      else pending.resolve(res.result);
      return;
    }
    // Notification.
    const note = msg as JsonRpcNotification;
    if (note.method === "sessionEvent") {
      const event = note.params as SessionEvent;
      for (const handler of this.sessionHandlers) handler(event);
    }
  }

  private onTransportDown(err: unknown): void {
    if (this.closed) return;
    // Reject every in-flight request so callers surface the failure.
    for (const [, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(err instanceof Error ? err : new Error(String(err)));
    }
    this.pending.clear();
    this.proc = null;
    this.emitConnection(false);
    this.scheduleRestart();
  }

  private scheduleRestart(): void {
    if (this.closed || this.restartTimer) return;
    const delay = this.backoff;
    this.backoff = Math.min(this.backoff * 2, BACKOFF_MAX_MS);
    this.restartTimer = setTimeout(() => {
      this.restartTimer = null;
      this.spawn();
    }, delay);
  }

  private emitConnection(connected: boolean): void {
    if (connected) this.backoff = BACKOFF_MIN_MS;
    for (const handler of this.connectionHandlers) handler(connected);
  }

  close(): void {
    this.closed = true;
    if (this.restartTimer) clearTimeout(this.restartTimer);
    for (const [, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(new Error("client closed"));
    }
    this.pending.clear();
    this.proc?.kill();
    this.proc = null;
  }

  // ── request plumbing ──────────────────────────────────────────────────────

  private call<T>(method: string, params?: unknown): Promise<T> {
    const proc = this.proc;
    if (!proc) {
      return Promise.reject(new Error("sidecar not connected"));
    }
    const id = this.nextId++;
    const req: JsonRpcRequest = { jsonrpc: "2.0", id, method };
    if (params !== undefined) req.params = params;
    const payload = JSON.stringify(req) + "\n";
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`rpc timeout: ${method}`));
      }, REQUEST_TIMEOUT_MS);
      this.pending.set(id, {
        resolve: resolve as (v: unknown) => void,
        reject,
        timer,
      });
      try {
        proc.stdin.write(payload);
        proc.stdin.flush();
      } catch (err) {
        this.pending.delete(id);
        clearTimeout(timer);
        reject(err);
      }
    });
  }

  // ── typed method wrappers ─────────────────────────────────────────────────

  hello(): Promise<HelloResult> {
    return this.call<HelloResult>("hello");
  }
  ls(): Promise<LsResult> {
    return this.call<LsResult>("ls");
  }
  show(slice: string): Promise<ShowResult> {
    return this.call<ShowResult>("show", { slice });
  }
  status(slice?: string): Promise<StatusEntry[]> {
    return this.call<StatusEntry[]>("status", slice ? { slice } : {});
  }
  prStack(slice: string): Promise<PrStackEntry[]> {
    return this.call<PrStackEntry[]>("prStack", { slice });
  }
  comments(slice: string): Promise<CommentsResult> {
    return this.call<CommentsResult>("comments", { slice });
  }
  conflicts(): Promise<ConflictsResult> {
    return this.call<ConflictsResult>("conflicts");
  }
  diff(params: {
    slice: string;
    scope: DiffScope;
    format: DiffFormat;
  }): Promise<DiffResult> {
    return this.call<DiffResult>("diff", params);
  }
  capture(params: { slice: string; lines: number }): Promise<CaptureResult> {
    return this.call<CaptureResult>("capture", params);
  }
  procs(slice?: string): Promise<ProcsResult> {
    return this.call<ProcsResult>("procs", slice ? { slice } : {});
  }

  // ── subscriptions ─────────────────────────────────────────────────────────

  onSessionEvent(handler: (event: SessionEvent) => void): () => void {
    this.sessionHandlers.add(handler);
    return () => this.sessionHandlers.delete(handler);
  }
  onConnectionChange(handler: (connected: boolean) => void): () => void {
    this.connectionHandlers.add(handler);
    return () => this.connectionHandlers.delete(handler);
  }
}

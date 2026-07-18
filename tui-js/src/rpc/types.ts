// TypeScript mirrors of the slis JSON shapes.
//
// The ls/show/status/prStack/comments/conflicts shapes are byte-for-byte the
// `slis <cmd> --json` output documented in docs/AGENT.md. The diff/capture/procs
// shapes are defined only in docs/plans/2026-07-18-js-tui-spike.md (the RPC
// surface v0) and are confirmed with the sidecar author separately.

export type SessionStatus = "none" | "running" | "waiting-input" | "done";

export type CiRollup = "pass" | "fail" | "pending";

export type ReviewDecision =
  | "APPROVED"
  | "CHANGES_REQUESTED"
  | "REVIEW_REQUIRED"
  | "";

// ── hello ──────────────────────────────────────────────────────────────────

export interface HelloResult {
  version: string;
  workspaceRoot: string;
  sessions: SessionsConfig;
}

// Resolved session config from workspace.yaml (see internal/rpcserver hello):
// harness/agent already have the "claude" defaults applied; layout is raw ("" →
// the front-end picks root-vs-repos); autostart has the legacy autostart_claude
// alias merged in.
export interface SessionsConfig {
  harness: string;
  agent: string;
  layout: string;
  autostart: boolean;
  // Configured editor binary (workspace.yaml sessions.editor). Empty/undefined
  // means auto-detect; when set, the e/o editor keys skip the picker.
  editor?: string;
}

// ── ls ─────────────────────────────────────────────────────────────────────

export interface SliceMember {
  repo: string;
  branch: string;
  worktree_path: string;
  tip_sha: string;
}

export interface Slice {
  name: string;
  base: string;
  active: boolean;
  stale: boolean;
  partial?: boolean;
  stack_id?: string;
  stack_order?: number;
  members: SliceMember[];
}

export interface SkippedWorktree {
  repo: string;
  path: string;
  branch: string;
  reason: string;
}

export interface RepoError {
  repo: string;
  error: string;
}

export interface Candidate {
  repo: string;
  path: string;
  branch: string;
  slice: string;
}

export interface MissingSlice {
  slice: string;
  repo: string;
  path: string;
  branch: string;
}

export interface LsResult {
  slices: Slice[];
  skipped?: SkippedWorktree[];
  repo_errors?: RepoError[];
  candidates?: Candidate[];
  missing?: MissingSlice[];
}

// ── show ───────────────────────────────────────────────────────────────────

export interface StackNode {
  name: string;
  depth: number;
  trunk: boolean;
  needs_restack: boolean;
}

export interface ShowMember extends SliceMember {
  stack?: StackNode[];
}

export interface ShowResult {
  name: string;
  base: string;
  active: boolean;
  members: ShowMember[];
}

// ── status ─────────────────────────────────────────────────────────────────

export interface StatusEntry {
  slice: string;
  status: SessionStatus;
}

// ── pr-stack ───────────────────────────────────────────────────────────────

export interface PrStackEntry {
  repo: string;
  branch: string;
  number?: number;
  url?: string;
  state?: string;
  title?: string;
  review_decision?: ReviewDecision;
  stack_order?: number;
  // CI check rollup, carried so a PR row can show a badge without a second
  // fetch. Omitted (all absent) for a branch with no PR.
  ci?: CiRollup;
  ci_pass?: number;
  ci_fail?: number;
  ci_pending?: number;
}

// ── ciLog (spec v0) ──────────────────────────────────────────────────────────

export interface CiLogRepo {
  repo: string;
  branch: string;
  log?: string; // safeterm-stripped `gh run view --log-failed` output
  error?: string; // set (log omitted) when no log is available for the repo
}

export interface CiLogResult {
  repos: CiLogRepo[];
}

// ── comments ───────────────────────────────────────────────────────────────

export interface PrComment {
  author: string;
  body: string;
  url: string;
  kind?: number; // 0 issue · 1 review · 2 inline (omitted by the sidecar when 0)
  context?: string; // review state, or path:line for inline comments (omitted when empty)
}

export interface RepoComments {
  pr: number;
  url: string;
  comments: PrComment[];
}

// slice -> repo -> RepoComments
export type CommentsResult = Record<string, Record<string, RepoComments>>;

// ── conflicts ──────────────────────────────────────────────────────────────

export interface ConflictOverlap {
  repo: string;
  path: string;
  slices: string[];
}

export interface ConflictsResult {
  overlaps: ConflictOverlap[];
  incomplete: string[];
}

// ── diff (spec v0) ─────────────────────────────────────────────────────────

export type DiffScope = "working" | "parent" | "trunk";
export type DiffFormat = "stat" | "patch" | "both";

export interface FileStat {
  path: string;
  added: number; // -1 for binary files
  deleted: number; // -1 for binary files
}

export interface DiffStat {
  files: FileStat[];
  added?: number;
  deleted?: number;
}

export interface DiffRepo {
  repo: string;
  branch: string;
  stat?: DiffStat | null;
  patch?: string | null;
  err?: string; // set (and stat/patch omitted) when this repo's diff failed
}

export interface DiffResult {
  repos: DiffRepo[];
}

// ── capture (spec v0) ──────────────────────────────────────────────────────

export interface CaptureResult {
  lines: string[];
}

// ── procs (spec v0) ────────────────────────────────────────────────────────

export interface ProcEntry {
  pid: number;
  ppid: number; // parent pid — for the wave-2 process-tree view
  cmd: string;
  cpu: number; // cumulative CPU % since start
  mem: number; // RSS MiB
}

export interface ProcSlice {
  slice: string;
  procs: ProcEntry[];
  totalCPU: number;
}

export interface ProcsResult {
  slices: ProcSlice[];
}

// ── session events (server → client notification) ────────────────────────────

export interface SessionEvent {
  slice: string;
  status: SessionStatus;
}

// ── the client contract shared by the real sidecar and the fake ──────────────

export interface RpcClient {
  hello(): Promise<HelloResult>;
  ls(): Promise<LsResult>;
  show(slice: string): Promise<ShowResult>;
  status(slice?: string): Promise<StatusEntry[]>;
  prStack(slice: string): Promise<PrStackEntry[]>;
  comments(slice: string): Promise<CommentsResult>;
  conflicts(): Promise<ConflictsResult>;
  diff(params: {
    slice: string;
    scope: DiffScope;
    format: DiffFormat;
  }): Promise<DiffResult>;
  capture(params: { slice: string; lines: number }): Promise<CaptureResult>;
  procs(slice?: string): Promise<ProcsResult>;
  ciLog(params: { slice: string; repo?: string }): Promise<CiLogResult>;

  /** Subscribe to live session-status changes. Returns an unsubscribe fn. */
  onSessionEvent(handler: (event: SessionEvent) => void): () => void;

  /** Fired when the underlying transport drops/reconnects (for a status line). */
  onConnectionChange(handler: (connected: boolean) => void): () => void;

  close(): void;
}

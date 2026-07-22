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
  // Selectable coding agents (name + argv). Resolved server-side to at least one
  // entry, but OPTIONAL here so an older sidecar that predates the field still
  // parses — the front-end then falls back to sessions.agent (see pickableAgents).
  agents?: AgentSpec[];
}

// One selectable coding agent from workspace.yaml sessions.agents (or the single
// resolved default). cmd is the launch argv; the front-end offers a picker when
// hello returns more than one.
export interface AgentSpec {
  name: string;
  cmd: string[];
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
  // Persisted display name from workspace.yaml sessions.default_agent.
  default_agent?: string;
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
  added?: number;
  deleted?: number;
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
  session_id?: string;
  cwd?: string;
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

// ── stack review: branchDiff / tree / file (F3) ──────────────────────────────

// One branch's committed diff against its Graphite stack parent (or trunk when
// it has none). Mirrors DiffRepo plus `parent` — the ref diffed against.
export interface BranchDiffResult {
  repo: string;
  branch: string;
  parent: string;
  stat?: DiffStat | null;
  patch?: string | null;
  err?: string;
}

export type TreeEntryType = "blob" | "tree" | "commit";

// One entry in a branch's tree listing. `name` is the leaf name within the
// listed directory (for lazy expansion). `size` is the blob byte size; -1 for
// trees and submodules.
export interface TreeEntry {
  name: string;
  type: TreeEntryType;
  size: number;
}

export interface TreeResult {
  repo: string;
  branch: string;
  path: string;
  entries: TreeEntry[];
}

// A file's content at a branch's revision. `content` is omitted (and `binary`
// true) for a binary file; text content is control-stripped server-side.
export interface FileResult {
  repo: string;
  branch: string;
  path: string;
  size: number;
  binary: boolean;
  content?: string;
}

// ── review (F2) ──────────────────────────────────────────────────────────────

// One pending inline-review comment — GitHub-review-style feedback awaiting
// delivery to a slice's agent. Mirrors `slis review list --json` (docs/AGENT.md
// §review). `line` and optional `end_line` are a 1-based range in the new
// (post-change) file; `branch` is the reviewed member/branch (filled at add
// time, may be ""); `hunk` is an optional selected diff/source excerpt.
export interface ReviewComment {
  id: string;
  slice: string;
  repo: string;
  branch?: string;
  file: string;
  line: number;
  end_line?: number;
  side?: "new" | "old";
  hunk?: string;
  body: string;
  author?: string;
  created_at: string;
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

  // Stack review (F3). An older sidecar answers -32601 (method not found); the
  // front-end catches that (isMethodNotFound) and hides the feature.
  branchDiff(params: {
    slice: string;
    repo: string;
    branch: string;
    format?: DiffFormat;
  }): Promise<BranchDiffResult>;
  tree(params: {
    slice: string;
    repo: string;
    branch: string;
    path?: string;
  }): Promise<TreeResult>;
  file(params: {
    slice: string;
    repo: string;
    branch: string;
    path: string;
    maxBytes?: number;
  }): Promise<FileResult>;

  // Pending inline-review comments (F2). Without a slice → every slice's; with
  // one → just that slice's. Read-only; add/rm/send stay CLI-only (see mutate).
  // An older sidecar answers -32601 (method not found) — callers feature-detect.
  reviews(params?: { slice?: string }): Promise<ReviewComment[]>;

  /** Subscribe to live session-status changes. Returns an unsubscribe fn. */
  onSessionEvent(handler: (event: SessionEvent) => void): () => void;

  /** Fired when the underlying transport drops/reconnects (for a status line). */
  onConnectionChange(
    handler: (connected: boolean, error?: Error) => void,
  ): () => void;

  close(): void;
}

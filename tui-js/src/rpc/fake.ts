// Fake RPC client: canned fixtures matching the docs/AGENT.md JSON shapes, so
// the UI is fully developable before `slis rpc` lands. Selected via SLIS_FAKE=1.
// It implements the exact same RpcClient interface as the real client, and even
// emits periodic sessionEvent notifications so live badge behaviour is testable.

import type {
  BranchDiffResult,
  CaptureResult,
  CiLogResult,
  CommentsResult,
  ConflictsResult,
  DiffResult,
  DiffScope,
  FileResult,
  FocusRequest,
  HelloResult,
  LsResult,
  ProcsResult,
  RpcClient,
  SessionEvent,
  SessionStatus,
  ShowResult,
  StatusEntry,
  PrStackEntry,
  ReviewComment,
  ReviewRun,
  TreeEntry,
  TreeResult,
} from "./types";
import { fakeReviewsList } from "./fakereviews";

const LATENCY_MS = 30; // pretend a small round-trip so loading states show

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), LATENCY_MS));
}

const LS: LsResult = {
  slices: [
    {
      name: "checkout",
      base: "",
      active: true,
      stale: true,
      stack_id: "web\u0000jonny/checkout-base",
      stack_order: 1,
      members: [
        {
          repo: "web",
          branch: "jonny/checkout",
          worktree_path: "/Users/jonny/work/.slis/worktrees/web/checkout",
          tip_sha: "f67b8a94c1d2e3f4",
        },
        {
          repo: "api",
          branch: "jonny/checkout",
          worktree_path: "/Users/jonny/work/.slis/worktrees/api/checkout",
          tip_sha: "a1b2c3d4e5f60718",
        },
      ],
    },
    {
      name: "payments",
      base: "",
      active: false,
      stale: false,
      partial: true,
      stack_id: "web\u0000jonny/checkout-base",
      stack_order: 2,
      members: [
        {
          repo: "web",
          branch: "jonny/payments",
          worktree_path: "/Users/jonny/work/.slis/worktrees/web/payments",
          tip_sha: "9988776655443322",
        },
      ],
    },
    {
      name: "refunds",
      base: "",
      active: false,
      stale: false,
      members: [
        {
          repo: "api",
          branch: "jonny/refunds",
          worktree_path: "/Users/jonny/work/.slis/worktrees/api/refunds",
          tip_sha: "1122334455667788",
        },
        {
          repo: "ops",
          branch: "jonny/refunds",
          worktree_path: "/Users/jonny/work/.slis/worktrees/ops/refunds",
          tip_sha: "8877665544332211",
        },
      ],
    },
    {
      name: "search-index",
      base: "",
      active: false,
      stale: false,
      members: [
        {
          repo: "api",
          branch: "jonny/search-index",
          worktree_path: "/Users/jonny/work/.slis/worktrees/api/search-index",
          tip_sha: "aabbccddeeff0011",
        },
      ],
    },
  ],
  candidates: [
    {
      repo: "web",
      path: "/Users/jonny/work/web-hotfix",
      branch: "jonny/hotfix",
      slice: "hotfix",
    },
  ],
};

const STATUS: Record<string, SessionStatus> = {
  checkout: "waiting-input",
  payments: "running",
  refunds: "done",
  "search-index": "none",
};

const PR_STACK: Record<string, PrStackEntry[]> = {
  checkout: [
    {
      repo: "web",
      branch: "jonny/checkout",
      number: 8107,
      url: "https://github.com/acme/web/pull/8107",
      state: "OPEN",
      title: "Checkout revamp",
      review_decision: "CHANGES_REQUESTED",
      stack_order: 1,
      ci: "fail",
      ci_pass: 5,
      ci_fail: 2,
      ci_pending: 0,
      comments: [
        {
          author: "reviewer",
          body: "This breaks the empty-cart case.",
          url: "https://github.com/acme/web/pull/8107#discussion_r1",
          kind: 2,
          context: "src/checkout/cart.tsx:12",
          path: "src/checkout/cart.tsx",
          line: 12,
          side: "RIGHT",
          diff_hunk: "@@ -10,7 +10,9 @@ export function Cart() {\n   const items = useCart();\n-  return <List items={items} />;\n+  const totals = useTotals(items);\n+  return <List items={items} totals={totals} />;\n }",
        },
        {
          author: "lead",
          body: "Please add coverage for the empty cart.",
          url: "https://github.com/acme/web/pull/8107#pullrequestreview-1",
          kind: 1,
          context: "CHANGES_REQUESTED",
        },
      ],
    },
    {
      repo: "api",
      branch: "jonny/checkout",
      number: 4412,
      url: "https://github.com/acme/api/pull/4412",
      state: "OPEN",
      title: "Checkout totals endpoint",
      review_decision: "APPROVED",
      stack_order: 1,
      ci: "pass",
      ci_pass: 7,
      ci_fail: 0,
      ci_pending: 0,
    },
  ],
  payments: [
    {
      repo: "web",
      branch: "jonny/payments",
      number: 8130,
      url: "https://github.com/acme/web/pull/8130",
      state: "OPEN",
      title: "Payments provider swap",
      review_decision: "REVIEW_REQUIRED",
      stack_order: 2,
      ci: "pending",
      ci_pass: 3,
      ci_fail: 0,
      ci_pending: 2,
    },
  ],
  refunds: [
    {
      repo: "api",
      branch: "jonny/refunds",
      number: 4450,
      url: "https://github.com/acme/api/pull/4450",
      state: "MERGED",
      title: "Refund ledger",
      review_decision: "APPROVED",
      stack_order: 1,
    },
    { repo: "ops", branch: "jonny/refunds" },
  ],
  "search-index": [{ repo: "api", branch: "jonny/search-index" }],
};

const SHOW: Record<string, ShowResult> = {
  checkout: {
    name: "checkout",
    base: "",
    active: true,
    members: [
      {
        repo: "web",
        branch: "jonny/checkout",
        worktree_path: "/Users/jonny/work/.slis/worktrees/web/checkout",
        tip_sha: "f67b8a94c1d2e3f4",
        stack: [
          { name: "main", depth: 0, trunk: true, needs_restack: false },
          {
            name: "jonny/checkout-base",
            depth: 1,
            trunk: false,
            needs_restack: false,
          },
          {
            name: "jonny/checkout",
            depth: 2,
            trunk: false,
            needs_restack: true,
          },
        ],
      },
      {
        repo: "api",
        branch: "jonny/checkout",
        worktree_path: "/Users/jonny/work/.slis/worktrees/api/checkout",
        tip_sha: "a1b2c3d4e5f60718",
        stack: [
          { name: "master", depth: 0, trunk: true, needs_restack: false },
          {
            name: "jonny/checkout",
            depth: 1,
            trunk: false,
            needs_restack: false,
          },
        ],
      },
    ],
  },
};

const CAPTURE: Record<string, string[]> = {
  checkout: [
    "● Running tests for checkout flow...",
    "  web: 42 passed, 0 failed",
    "  api: 18 passed, 1 failed",
    "",
    "I need your input: the refund path has two possible designs.",
    "Which should I implement? (1) synchronous (2) queued",
    "› _",
  ],
  payments: [
    "● Refactoring provider adapter...",
    "  editing src/payments/provider.ts",
    "  running typecheck...",
  ],
  refunds: ["✓ Done. All checks passing. Opened PR #4450.", "› _"],
};

const DIFF_STAT: DiffResult = {
  repos: [
    {
      repo: "web",
      branch: "jonny/checkout",
      stat: {
        files: [
          { path: "src/checkout/cart.tsx", added: 84, deleted: 12 },
          { path: "src/checkout/totals.ts", added: 31, deleted: 4 },
          { path: "src/checkout/__tests__/cart.test.tsx", added: 56, deleted: 0 },
        ],
        added: 171,
        deleted: 16,
      },
      patch: [
        "diff --git a/src/checkout/cart.tsx b/src/checkout/cart.tsx",
        "index 1a2b3c4..5d6e7f8 100644",
        "--- a/src/checkout/cart.tsx",
        "+++ b/src/checkout/cart.tsx",
        "@@ -10,7 +10,9 @@ export function Cart() {",
        "   const items = useCart();",
        "-  return <List items={items} />;",
        "+  const totals = useTotals(items);",
        "+  return <List items={items} totals={totals} />;",
        " }",
      ].join("\n"),
    },
    {
      repo: "api",
      branch: "jonny/checkout",
      stat: {
        files: [{ path: "internal/checkout/totals.go", added: 47, deleted: 3 }],
        added: 47,
        deleted: 3,
      },
      patch: [
        "diff --git a/internal/checkout/totals.go b/internal/checkout/totals.go",
        "@@ -1,3 +1,5 @@",
        "+// Totals computes the order total including tax.",
        "+func Totals(o Order) Money { return o.Sum().WithTax() }",
      ].join("\n"),
    },
  ],
};

// Fake failing-CI logs keyed slice → repo (only repos whose fixture ci is
// "fail" get a log; others report a "no failing CI run" error, mirroring
// forge.FailedLog).
const CI_LOG: Record<string, Record<string, string>> = {
  checkout: {
    web: [
      "test  Run tests",
      "test  ● checkout › totals › applies tax",
      "test    expected 1210, received 1000",
      "test      at src/checkout/__tests__/totals.test.ts:42:18",
      "test  2 failed, 40 passed",
      "test  Process completed with exit code 1.",
    ].join("\n"),
  },
};

// Per-branch diff-vs-parent fixtures, keyed `repo:branch`. Lets the cockpit's
// branch-review surface show a distinct diff for each node in the stack.
const BRANCH_DIFF: Record<string, BranchDiffResult> = {
  "web:jonny/checkout-base": {
    repo: "web",
    branch: "jonny/checkout-base",
    parent: "main",
    stat: {
      files: [{ path: "src/checkout/types.ts", added: 22, deleted: 0 }],
      added: 22,
      deleted: 0,
    },
    patch: [
      "diff --git a/src/checkout/types.ts b/src/checkout/types.ts",
      "new file mode 100644",
      "--- /dev/null",
      "+++ b/src/checkout/types.ts",
      "@@ -0,0 +1,3 @@",
      "+export interface Cart {",
      "+  items: Item[];",
      "+}",
    ].join("\n"),
  },
  "web:jonny/checkout": {
    repo: "web",
    branch: "jonny/checkout",
    parent: "jonny/checkout-base",
    stat: {
      files: [{ path: "src/checkout/cart.tsx", added: 84, deleted: 12 }],
      added: 84,
      deleted: 12,
    },
    patch: [
      "diff --git a/src/checkout/cart.tsx b/src/checkout/cart.tsx",
      "@@ -10,7 +10,9 @@ export function Cart() {",
      "   const items = useCart();",
      "-  return <List items={items} />;",
      "+  const totals = useTotals(items);",
      "+  return <List items={items} totals={totals} />;",
      " }",
    ].join("\n"),
  },
  "api:jonny/checkout": {
    repo: "api",
    branch: "jonny/checkout",
    parent: "master",
    stat: {
      files: [{ path: "internal/checkout/totals.go", added: 47, deleted: 3 }],
      added: 47,
      deleted: 3,
    },
    patch: [
      "diff --git a/internal/checkout/totals.go b/internal/checkout/totals.go",
      "@@ -1,3 +1,5 @@",
      "+// Totals computes the order total including tax.",
      "+func Totals(o Order) Money { return o.Sum().WithTax() }",
    ].join("\n"),
  },
};

// A small tree keyed by directory path (empty = root), reused for every branch
// of the checkout slice so the lazy file-tree browser is navigable.
const TREE: Record<string, TreeEntry[]> = {
  "": [
    { name: "src", type: "tree", size: -1 },
    { name: "README.md", type: "blob", size: 128 },
  ],
  src: [
    { name: "checkout", type: "tree", size: -1 },
    { name: "app.ts", type: "blob", size: 512 },
  ],
  "src/checkout": [
    { name: "cart.tsx", type: "blob", size: 2048 },
    { name: "totals.ts", type: "blob", size: 640 },
    { name: "logo.png", type: "blob", size: 8192 },
  ],
};

const FILE_CONTENT: Record<string, string> = {
  "README.md": "# Checkout\n\nThe checkout flow.\n",
  "src/app.ts": "export const app = () => {\n  return 42;\n};\n",
  "src/checkout/cart.tsx": [
    "import { useCart, useTotals } from './hooks';",
    "",
    "export function Cart() {",
    "  const items = useCart();",
    "  const totals = useTotals(items);",
    "  return <List items={items} totals={totals} />;",
    "}",
  ].join("\n"),
  "src/checkout/totals.ts": "export const total = (n: number) => n * 1.21;\n",
};

const PROCS: ProcsResult = {
  slices: [
    {
      slice: "checkout",
      totalCPU: 143.2,
      procs: [
        { pid: 40122, ppid: 40100, cmd: "node (vite dev)", cpu: 98.4, mem: 512.0 },
        { pid: 40155, ppid: 40100, cmd: "go run ./cmd/api", cpu: 41.1, mem: 210.5 },
        { pid: 40130, ppid: 40122, cmd: "esbuild", cpu: 3.7, mem: 88.2 },
      ],
    },
  ],
};

export class FakeRpcClient implements RpcClient {
  private readonly sessionHandlers = new Set<(e: SessionEvent) => void>();
  private readonly focusHandlers = new Set<(request: FocusRequest) => void>();
  private readonly connectionHandlers = new Set<(c: boolean) => void>();
  private readonly statusMap: Record<string, SessionStatus> = { ...STATUS };
  private ticker: ReturnType<typeof setInterval> | null = null;

  constructor() {
    // Flip a couple of statuses on a slow loop so live badges are observable.
    const cycle: Array<[string, SessionStatus]> = [
      ["payments", "waiting-input"],
      ["search-index", "running"],
      ["payments", "running"],
      ["search-index", "none"],
    ];
    let i = 0;
    this.ticker = setInterval(() => {
      const entry = cycle[i % cycle.length]!;
      i++;
      const [slice, status] = entry;
      this.statusMap[slice] = status;
      for (const handler of this.sessionHandlers) handler({ slice, status });
    }, 4000);
  }

  hello(): Promise<HelloResult> {
    return delay({
      version: "fake-0.0.0",
      workspaceRoot: "/Users/jonny/work",
      sessions: { harness: "claude", agent: "claude", layout: "", autostart: false },
      agents: [
        { name: "claude", cmd: ["claude"] },
        { name: "codex", cmd: ["codex"] },
      ],
    });
  }
  ls(): Promise<LsResult> {
    return delay(structuredClone(LS));
  }
  show(slice: string): Promise<ShowResult> {
    const s = SHOW[slice];
    if (s) return delay(structuredClone(s));
    // Synthesize a minimal show from ls for slices without a fixture.
    const found = LS.slices.find((x) => x.name === slice);
    if (!found) return Promise.reject(new Error(`slice not found: ${slice}`));
    return delay({
      name: found.name,
      base: found.base,
      active: found.active,
      members: found.members.map((m) => ({ ...m, stack: [] })),
    });
  }
  status(slice?: string): Promise<StatusEntry[]> {
    if (slice) {
      return delay([{ slice, status: this.statusMap[slice] ?? "none" }]);
    }
    return delay(
      Object.entries(this.statusMap).map(([s, st]) => ({ slice: s, status: st })),
    );
  }
  prStack(slice: string): Promise<PrStackEntry[]> {
    return delay(structuredClone(PR_STACK[slice] ?? []));
  }
  comments(slice: string): Promise<CommentsResult> {
    if (slice === "checkout") {
      return delay({
        checkout: {
          web: {
            pr: 8107,
            url: "https://github.com/acme/web/pull/8107",
            comments: [
              {
                author: "reviewer",
                body: "This breaks the empty-cart case.",
                url: "https://github.com/acme/web/pull/8107#discussion_r1",
                kind: 2,
                context: "src/checkout/cart.tsx:14",
              },
            ],
          },
        },
      });
    }
    return delay({});
  }
  conflicts(): Promise<ConflictsResult> {
    return delay({
      overlaps: [
        {
          repo: "web",
          path: "src/checkout/totals.ts",
          slices: ["checkout", "payments"],
        },
      ],
      incomplete: [],
    });
  }
  diff(params: { slice: string; scope: DiffScope }): Promise<DiffResult> {
    if (params.slice === "checkout") return delay(structuredClone(DIFF_STAT));
    return delay({ repos: [] });
  }
  capture(params: { slice: string; lines: number }): Promise<CaptureResult> {
    const lines = CAPTURE[params.slice] ?? [];
    return delay({ lines: lines.slice(-params.lines) });
  }
  procs(slice?: string): Promise<ProcsResult> {
    if (slice) {
      return delay({ slices: PROCS.slices.filter((s) => s.slice === slice) });
    }
    return delay(structuredClone(PROCS));
  }
  ciLog(params: { slice: string; repo?: string }): Promise<CiLogResult> {
    const rows = PR_STACK[params.slice] ?? [];
    const targets = params.repo
      ? rows.filter((r) => r.repo === params.repo)
      : rows;
    const logs = CI_LOG[params.slice] ?? {};
    return delay({
      repos: targets.map((r) => {
        if (r.number === undefined) {
          return { repo: r.repo, branch: r.branch, error: "no open PR for this branch" };
        }
        const log = logs[r.repo];
        return log
          ? { repo: r.repo, branch: r.branch, log }
          : { repo: r.repo, branch: r.branch, error: "no failing CI run found" };
      }),
    });
  }

  branchDiff(params: {
    slice: string;
    repo: string;
    branch: string;
  }): Promise<BranchDiffResult> {
    const hit = BRANCH_DIFF[`${params.repo}:${params.branch}`];
    if (hit) return delay(structuredClone(hit));
    // Unknown branch (e.g. trunk): an empty diff against itself.
    return delay({
      repo: params.repo,
      branch: params.branch,
      parent: params.branch,
      stat: { files: [], added: 0, deleted: 0 },
      patch: "",
    });
  }
  tree(params: {
    slice: string;
    repo: string;
    branch: string;
    path?: string;
  }): Promise<TreeResult> {
    const path = params.path ?? "";
    return delay({
      repo: params.repo,
      branch: params.branch,
      path,
      entries: structuredClone(TREE[path] ?? []),
    });
  }
  file(params: {
    slice: string;
    repo: string;
    branch: string;
    path: string;
    maxBytes?: number;
  }): Promise<FileResult> {
    const content = FILE_CONTENT[params.path];
    if (content === undefined) {
      // Treat anything without canned text as a binary blob (e.g. logo.png).
      return delay({
        repo: params.repo,
        branch: params.branch,
        path: params.path,
        size: 8192,
        binary: true,
      });
    }
    return delay({
      repo: params.repo,
      branch: params.branch,
      path: params.path,
      size: content.length,
      binary: false,
      content,
    });
  }

  reviews(params?: { slice?: string }): Promise<ReviewComment[]> {
    return delay(fakeReviewsList(params?.slice));
  }

  reviewRuns(params?: { slice?: string; includeMessages?: boolean }): Promise<ReviewRun[]> {
    const runs: ReviewRun[] = [
      {
        id: "review-checkout-codex",
        slice: "checkout",
        agent: "Codex",
        window: "review-codex",
        status: "findings",
        finding_count: 2,
        created_at: "2026-07-31T10:00:00Z",
        updated_at: "2026-07-31T10:04:00Z",
        messages: [
          {
            id: "message-1",
            role: "user",
            body: "Review the complete stack and report any concrete risks.",
            created_at: "2026-07-31T10:00:00Z",
          },
          {
            id: "message-2",
            role: "reviewer",
            body: "I found two correctness risks in the checkout state transition. Both findings were delivered to the working agent.",
            created_at: "2026-07-31T10:04:00Z",
          },
        ],
      },
    ];
    const filtered = runs.filter((run) => !params?.slice || run.slice === params.slice);
    return delay(
      params?.includeMessages
        ? filtered
        : filtered.map(({ messages: _, ...run }) => run),
    );
  }

  onSessionEvent(handler: (event: SessionEvent) => void): () => void {
    this.sessionHandlers.add(handler);
    return () => this.sessionHandlers.delete(handler);
  }
  onFocusRequest(handler: (request: FocusRequest) => void): () => void {
    this.focusHandlers.add(handler);
    return () => this.focusHandlers.delete(handler);
  }
  ackFocus(_id: string): void {}
  onConnectionChange(handler: (connected: boolean) => void): () => void {
    this.connectionHandlers.add(handler);
    // Fake is always "connected".
    queueMicrotask(() => handler(true));
    return () => this.connectionHandlers.delete(handler);
  }
  close(): void {
    if (this.ticker) clearInterval(this.ticker);
    this.ticker = null;
  }
}

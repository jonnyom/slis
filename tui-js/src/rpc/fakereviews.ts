// Shared in-memory pending-review store for SLIS_FAKE. Both the fake RPC client
// (the read-only `reviews` method) and the fake review mutations (add/rm/send in
// mutate.ts) operate on this single module-level store, so the whole inline
// review loop — comment → list → gutter marker → send → toast — is exercisable
// headlessly with no real `slis` binary.

import type { ReviewComment } from "./types";

// Seeded with one comment on the checkout slice's cart.tsx change (new line 12 =
// the `+  return <List items={items} totals={totals} />;` line in the fixture
// diff), so a fresh fake session shows the breadcrumb badge and a gutter marker
// immediately.
const SEED: ReviewComment[] = [
  {
    id: "seed0001abcd",
    slice: "checkout",
    repo: "web",
    branch: "jonny/checkout",
    file: "src/checkout/cart.tsx",
    line: 12,
    hunk: "+  const totals = useTotals(items);\n+  return <List items={items} totals={totals} />;",
    body: "pass the memoized totals in here instead of recomputing",
    created_at: "2026-07-18T13:42:38Z",
  },
];

let store: ReviewComment[] = SEED.map((c) => ({ ...c }));
let counter = 1;

function nextId(): string {
  const n = counter++;
  return "fake" + n.toString(16).padStart(8, "0");
}

// Deterministic order matching the CLI: slice, repo, file, line, id.
function sortComments(a: ReviewComment, b: ReviewComment): number {
  return (
    a.slice.localeCompare(b.slice) ||
    a.repo.localeCompare(b.repo) ||
    a.file.localeCompare(b.file) ||
    a.line - b.line ||
    a.id.localeCompare(b.id)
  );
}

export function fakeReviewsList(slice?: string): ReviewComment[] {
  return store
    .filter((c) => !slice || c.slice === slice)
    .sort(sortComments)
    .map((c) => ({ ...c }));
}

export interface FakeReviewAddInput {
  slice: string;
  repo: string;
  branch?: string;
  file: string;
  line: number;
  endLine?: number;
  side?: "new" | "old";
  hunk?: string;
  body: string;
}

export function fakeReviewAdd(input: FakeReviewAddInput): ReviewComment {
  const comment: ReviewComment = {
    id: nextId(),
    slice: input.slice,
    repo: input.repo,
    branch: input.branch ?? "",
    file: input.file,
    line: input.line,
    end_line: input.endLine,
    side: input.side,
    hunk: input.hunk,
    body: input.body,
    created_at: new Date().toISOString(),
  };
  store.push(comment);
  return comment;
}

export function fakeReviewRm(slice: string, id: string): boolean {
  const before = store.length;
  store = store.filter((c) => !(c.slice === slice && c.id === id));
  return store.length < before;
}

// Delivering clears the slice's pending batch (the fake always has a session).
export function fakeReviewSend(slice: string): number {
  const n = store.filter((c) => c.slice === slice).length;
  if (n > 0) store = store.filter((c) => c.slice !== slice);
  return n;
}

// Test-only reset so unit tests start from a known state.
export function resetFakeReviews(): void {
  store = SEED.map((c) => ({ ...c }));
  counter = 1;
}

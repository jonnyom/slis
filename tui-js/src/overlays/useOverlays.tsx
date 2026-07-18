// useOverlays owns the single active overlay, its keyboard routing, and the
// shared one-shot mutation runner (working → result, then refresh). app.go runs
// a priority chain over many independent overlay flags; here a single nullable
// discriminated union makes the priority implicit — whichever overlay is set is
// the one that handles keys and renders. Browser / cockpit stay disabled while
// `active` is true, so exactly one keyboard owner is live at a time.

import { useCallback, useRef, useState, type ReactNode } from "react";
import { useKeyboard } from "@opentui/react";
import type { KeyEvent } from "@opentui/core";
import type {
  AgentSpec,
  Candidate,
  ConflictsResult,
  ReviewComment,
  RpcClient,
} from "../rpc/types";
import type { CommentContext } from "../review/context";
import { clampReviewSel } from "../review/context";
import { quickPickIndex } from "../term/agentpick";
import type { EditorSpec } from "../editor/detect";
import {
  activate,
  activateStash,
  adoptBranch,
  ciRerunSlice,
  copyToClipboard,
  deactivate,
  editorSet,
  editPath,
  editRepo,
  editSlice,
  fixCiSlice,
  groupSlices,
  ignoreCandidate,
  importCandidate,
  isFake,
  mergeSlice,
  mutationArgv,
  mutationRoute,
  openUrl,
  prStackMarkdown,
  removeSlice,
  restackSlice,
  reviewAdd,
  reviewRm,
  reviewSend,
  submitSlice,
  summarySlice,
  syncSlice,
  ungroupSlice,
  type MutateResult,
} from "../rpc/mutate";
import { editText } from "./textinput";
import {
  CandidatesOverlay,
  CiRerunOverlay,
  ConflictRadarOverlay,
  AgentPickerOverlay,
  CommentComposerOverlay,
  CreateOverlay,
  EditorPickerOverlay,
  GroupOverlay,
  RemoveOverlay,
  ResultOverlay,
  ReviewListOverlay,
  StackActionsOverlay,
  SummaryOverlay,
  SwapOverlay,
  WorkingOverlay,
} from "./overlays";
import { AdoptOverlay } from "./adopt";
import { normalizeKeyName } from "../util/keys";
import { Help } from "../components/help";
import type { BadgeState, ResultStatus } from "../theme";

type Overlay =
  | { kind: "help" }
  | { kind: "swap"; slice: string; active: boolean; dirty: boolean }
  | { kind: "stack"; slices: string[]; conflictWith: string[] }
  | { kind: "remove"; slices: string[] }
  | { kind: "ciRerun"; slice: string }
  | { kind: "create"; text: string }
  | { kind: "adopt"; text: string }
  | { kind: "group"; slices: string[]; text: string; onDone: () => void }
  | { kind: "candidates"; items: Candidate[]; sel: number }
  | {
      kind: "editorPicker";
      editors: EditorSpec[];
      sel: number;
      slice: string;
      repo?: string;
      path?: string;
      line?: number;
    }
  | { kind: "agentPicker"; agents: AgentSpec[]; sel: number; slice: string; onPick: (agent: AgentSpec) => void }
  | { kind: "conflicts"; scroll: number }
  | { kind: "summary"; slice: string; ai: boolean; loading: boolean; text: string; scroll: number }
  | { kind: "comment"; ctx: CommentContext; text: string; onAdded: () => void }
  | {
      kind: "review";
      slice: string;
      comments: ReviewComment[] | null;
      sel: number;
      confirmSend: boolean;
      onChanged: () => void;
    }
  | { kind: "working"; text: string }
  | { kind: "result"; title: string; body: string; status: ResultStatus }
  | null;

export interface OverlayApi {
  active: boolean;
  node: ReactNode;
  view: "browser" | "cockpit";
  // modal openers
  help(): void;
  swap(slice: string, active: boolean): void;
  stack(slices: string[], conflictWith: string[]): void;
  remove(slices: string[]): void;
  ciRerun(slice: string): void;
  fixCi(slice: string): void;
  create(): void;
  adopt(): void;
  candidates(items: Candidate[]): void;
  group(slices: string[], onDone: () => void): void;
  conflictRadar(): void;
  summary(slice: string, ai: boolean): void;
  // F2 inline review: comment composer + pending-review overlay.
  comment(ctx: CommentContext, onAdded: () => void): void;
  review(slice: string, onChanged: () => void): void;
  editor(slice: string, repo?: string, path?: string, line?: number): void;
  agentPicker(
    slice: string,
    agents: AgentSpec[],
    onPick: (agent: AgentSpec) => void,
    preferredAgent?: string,
  ): void;
  info(title: string, body: string): void;
  error(title: string, body: string): void;
  // immediate actions (still funnel through the shared runner)
  ungroup(slice: string): void;
  yankDiff(text: string): void;
  yankPrStack(slice: string): void;
  openPr(url: string): void;
}

export interface UseOverlaysArgs {
  refresh: () => void;
  conflicts: ConflictsResult | null;
  view: "browser" | "cockpit";
  height: number;
  client: RpcClient;
  // Editors found on PATH + the configured editor (workspace.yaml sessions.editor).
  // Drive the editor open flow: skip the picker when one is configured or exactly
  // one is available.
  editors: EditorSpec[];
  configuredEditor?: string;
  // Open an interactive mutation in a PTY terminal tab (submit/sync/merge/adopt/
  // fix-ci). Provided by the app, which owns the terminal layer.
  runInteractive: (argv: string[], title: string) => void;
  // Push a transient toast (spec §3.5) — quick confirmations that don't deserve
  // a modal (yank / swap done).
  toast: (message: string, state?: BadgeState) => void;
  // Kick off a non-blocking slice create (spec D2). The app owns the create
  // state machine + ambient header spinner; the overlay only collects the name.
  startCreate: (name: string) => void;
}

async function runSequential(
  slices: string[],
  fn: (slice: string) => Promise<MutateResult>,
): Promise<MutateResult> {
  const parts: string[] = [];
  let worst = 0;
  for (const slice of slices) {
    const r = await fn(slice);
    const body = (r.stdout + (r.stderr ? "\n" + r.stderr : "")).trim();
    parts.push(`── ${slice} ──\n${body || "(no output)"}`);
    if (r.code !== 0 && worst === 0) worst = r.code;
  }
  return { code: worst, stdout: parts.join("\n\n"), stderr: "" };
}

export function useOverlays(args: UseOverlaysArgs): OverlayApi {
  const {
    refresh,
    conflicts,
    view,
    height,
    client,
    editors,
    configuredEditor,
    runInteractive,
    toast,
    startCreate,
  } = args;
  const [overlay, setOverlay] = useState<Overlay>(null);
  const helpReturn = useRef<Overlay>(null);
  const close = useCallback(() => {
    helpReturn.current = null;
    setOverlay(null);
  }, []);
  const openHelp = useCallback(() => {
    helpReturn.current = null;
    setOverlay({ kind: "help" });
  }, []);

  // Working → Result → refresh. A `successToast` routes a clean run to a
  // transient toast (and closes the modal) instead of a Result overlay; a
  // failure always falls back to the Result overlay so the output is readable.
  const runMutation = useCallback(
    (title: string, fn: () => Promise<MutateResult>, opts?: { successToast?: string }) => {
      setOverlay({ kind: "working", text: title + "…" });
      fn().then(
        (res) => {
          const ok = res.code === 0;
          if (ok && opts?.successToast) {
            toast(opts.successToast, "ci-pass");
            close();
          } else {
            setOverlay({
              kind: "result",
              title: ok ? title : title + " — failed",
              body: (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
              status: ok ? "success" : "failure",
            });
          }
          refresh();
        },
        (err) =>
          setOverlay({
            kind: "result",
            title: title + " — failed",
            body: String(err),
            status: "failure",
          }),
      );
    },
    [refresh, toast, close],
  );

  // No modal at all: run in the background, confirm success with a toast, and
  // only raise a Result overlay when it fails. For quick clipboard writes.
  const runQuiet = useCallback(
    (
      successToast: string,
      fn: () => Promise<MutateResult>,
      failureTitle = "Failed",
    ) => {
      close();
      fn().then(
        (res) => {
          if (res.code === 0) toast(successToast, "ci-pass");
          else
            setOverlay({
              kind: "result",
              title: failureTitle,
              body: (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
              status: "failure",
            });
        },
        (err) =>
          setOverlay({ kind: "result", title: failureTitle, body: String(err), status: "failure" }),
      );
    },
    [toast, close],
  );

  // Route a mutation by its command kind: interactive commands (submit/sync/
  // merge/adopt/fix-ci) run in a PTY tab so the user can answer prompts / drive
  // the agent; everything else runs captured through runMutation. Under
  // SLIS_FAKE there is no real PTY, so we keep the captured (fake) path.
  const runMutationRouted = useCallback(
    (title: string, command: string, cmdArgs: string[], captured: () => Promise<MutateResult>) => {
      if (!isFake() && mutationRoute(command) === "interactive") {
        close(); // dismiss the triggering overlay; the tab takes over
        runInteractive(mutationArgv(command, cmdArgs), title);
        return;
      }
      runMutation(title, captured);
    },
    [close, runInteractive, runMutation],
  );

  const openSummary = useCallback((slice: string, ai: boolean) => {
    setOverlay({ kind: "summary", slice, ai, loading: true, text: "", scroll: 0 });
    summarySlice(slice, ai).then(
      (r) =>
        setOverlay((o) =>
          o && o.kind === "summary" && o.slice === slice
            ? { ...o, loading: false, text: r.stdout || r.stderr || "(no output)" }
            : o,
        ),
      (err) =>
        setOverlay((o) =>
          o && o.kind === "summary" ? { ...o, loading: false, text: String(err) } : o,
        ),
    );
  }, []);

  // Re-fetch a slice's pending comments into an open review overlay (after a
  // delete). Read-only; the overlay stays put if the user has moved on.
  const reloadReview = useCallback(
    (slice: string) => {
      client.reviews({ slice }).then(
        (rows) =>
          setOverlay((o) =>
            o && o.kind === "review" && o.slice === slice
              ? { ...o, comments: rows, sel: clampReviewSel(o.sel, rows.length) }
              : o,
          ),
        () =>
          setOverlay((o) =>
            o && o.kind === "review" && o.slice === slice ? { ...o, comments: [] } : o,
          ),
      );
    },
    [client],
  );

  const openReview = useCallback(
    (slice: string, onChanged: () => void) => {
      setOverlay({ kind: "review", slice, comments: null, sel: 0, confirmSend: false, onChanged });
      reloadReview(slice);
    },
    [reloadReview],
  );

  // Open the swap confirm. For a swap-IN we asynchronously probe the working
  // tree so the overlay can offer [s] activate --stash when it is dirty.
  const openSwap = useCallback(
    (slice: string, active: boolean) => {
      setOverlay({ kind: "swap", slice, active, dirty: false });
      if (active) return; // swap-OUT never stashes
      client.diff({ slice, scope: "working", format: "stat" }).then(
        (d) => {
          const dirty = d.repos.some((r) => (r.stat?.files.length ?? 0) > 0);
          if (dirty)
            setOverlay((o) =>
              o && o.kind === "swap" && o.slice === slice ? { ...o, dirty: true } : o,
            );
        },
        () => {},
      );
    },
    [client],
  );

  // Open a path/repo/slice in the editor. Successful GUI launches are quiet:
  // dismiss immediately and confirm with a toast; only failures deserve a
  // blocking result overlay. A configured editor (or a lone available one)
  // opens immediately; several
  // configured editor (or a lone available one) opens immediately; several
  // detected and none configured raises the picker (which persists the choice).
  const runEdit = useCallback(
    (
      slice: string,
      repo: string | undefined,
      path: string | undefined,
      line: number | undefined,
      persistBin?: string,
    ) => {
      const label = path
        ? `Open ${path}${line ? `:${line}` : ""} in editor`
        : repo
          ? `Open ${repo} in editor`
          : "Open editor";
      const bin = persistBin ?? configuredEditor ?? (editors.length === 1 ? editors[0]?.bin : undefined);
      const editorName = editors.find((editor) => editor.bin === bin)?.name ?? bin ?? "editor";
      const target = path
        ? `${path.split("/").at(-1) ?? path}${line ? `:${line}` : ""}`
        : repo ?? slice;
      runQuiet(
        `Opened ${target} in ${editorName}`,
        async () => {
          if (persistBin) {
            const set = await editorSet(persistBin);
            if (set.code !== 0) return set;
          }
          if (path && repo) return editPath(slice, repo, path, line);
          return repo ? editRepo(slice, repo) : editSlice(slice);
        },
        label + " — failed",
      );
    },
    [configuredEditor, editors, runQuiet],
  );

  const openEditor = useCallback(
    (slice: string, repo?: string, path?: string, line?: number) => {
      if (configuredEditor || editors.length === 1) {
        runEdit(slice, repo, path, line);
        return;
      }
      if (editors.length === 0) {
        setOverlay({
          kind: "result",
          title: "No editor found",
          body: "install cursor / code / zed, or run `slis editor set <bin>`.",
          status: "failure",
        });
        return;
      }
      setOverlay({ kind: "editorPicker", editors, sel: 0, slice, repo, path, line });
    },
    [configuredEditor, editors, runEdit],
  );

  const api: OverlayApi = {
    active: overlay !== null,
    view,
    node: renderOverlay(overlay, conflicts, view, height),
    help: openHelp,
    swap: openSwap,
    stack: (slices, conflictWith) => setOverlay({ kind: "stack", slices, conflictWith }),
    remove: (slices) => setOverlay({ kind: "remove", slices }),
    ciRerun: (slice) => setOverlay({ kind: "ciRerun", slice }),
    fixCi: (slice) =>
      runMutationRouted("Fix CI " + slice, "fix-ci", [slice], () => fixCiSlice(slice)),
    create: () => setOverlay({ kind: "create", text: "" }),
    adopt: () => setOverlay({ kind: "adopt", text: "" }),
    candidates: (items) => setOverlay({ kind: "candidates", items, sel: 0 }),
    group: (slices, onDone) => setOverlay({ kind: "group", slices, text: "", onDone }),
    conflictRadar: () => setOverlay({ kind: "conflicts", scroll: 0 }),
    summary: openSummary,
    comment: (ctx, onAdded) => setOverlay({ kind: "comment", ctx, text: "", onAdded }),
    review: openReview,
    editor: openEditor,
    agentPicker: (slice, agents, onPick, preferredAgent) => {
      const preferred = agents.findIndex((agent) => agent.name === preferredAgent);
      setOverlay({ kind: "agentPicker", slice, agents, sel: preferred >= 0 ? preferred : 0, onPick });
    },
    info: (title, body) => setOverlay({ kind: "result", title, body, status: "warn" }),
    error: (title, body) => setOverlay({ kind: "result", title, body, status: "failure" }),
    ungroup: (slice) => runMutation("Ungroup " + slice, () => ungroupSlice(slice)),
    yankDiff: (text) => runQuiet("Copied diff to clipboard", () => copyToClipboard(text)),
    yankPrStack: (slice) =>
      runQuiet("Copied PR-stack markdown", async () => {
        const md = await prStackMarkdown(slice);
        if (md.code !== 0) return md;
        return copyToClipboard(md.stdout || "(no PRs)");
      }),
    openPr: (url) => runQuiet("Opened PR in browser", () => openUrl(url)),
  };

  useKeyboard((key: KeyEvent) => {
    if (!overlay) return;
    const name = normalizeKeyName(key);
    const isEnter = name === "return" || name === "enter";
    const isCancel = name === "escape";

    switch (overlay.kind) {
      case "help":
        if (name === "?" || isCancel || name === "q") {
          const previous = helpReturn.current;
          helpReturn.current = null;
          setOverlay(previous);
        }
        return;
      case "working":
        return;
      case "result":
        if (isEnter || isCancel || name === "q") close();
        return;
      case "swap":
        if (name === "y" || isEnter)
          runMutation(
            overlay.active ? "Swap out" : "Swap in",
            () => (overlay.active ? deactivate(overlay.slice) : activate(overlay.slice)),
            {
              successToast: overlay.active
                ? `Swapped out ${overlay.slice}`
                : `Swapped in ${overlay.slice}`,
            },
          );
        else if (name === "s" && overlay.dirty && !overlay.active)
          runMutation("Swap in (stash dirty)", () => activateStash(overlay.slice), {
            successToast: `Swapped in ${overlay.slice}`,
          });
        else if (name === "n" || isCancel) close();
        return;
      case "stack": {
        const first = overlay.slices[0] ?? "";
        if (name === "r")
          runMutation("Restack " + overlay.slices.join(", "), () =>
            runSequential(overlay.slices, restackSlice),
          );
        else if (name === "p")
          runMutationRouted("Submit " + first, "submit", [first], () => submitSlice(first));
        else if (name === "m")
          runMutationRouted("Merge " + first, "merge", [first], () => mergeSlice(first));
        else if (name === "s")
          runMutationRouted("Sync " + first, "sync", [first], () => syncSlice(first));
        else if (name === "n" || isCancel) close();
        return;
      }
      case "remove":
        if (name === "y")
          runMutation("Clear " + overlay.slices.join(", "), () =>
            runSequential(overlay.slices, (s) => removeSlice(s, false)),
          );
        else if (name === "f")
          runMutation("Force clear " + overlay.slices.join(", "), () =>
            runSequential(overlay.slices, (s) => removeSlice(s, true)),
          );
        else if (name === "n" || isCancel) close();
        return;
      case "ciRerun":
        if (name === "y" || isEnter)
          runMutation("Re-run CI " + overlay.slice, () => ciRerunSlice(overlay.slice));
        else if (name === "n" || isCancel) close();
        return;
      case "create":
        if (isCancel) close();
        else if (isEnter) {
          const nm = overlay.text.trim();
          close();
          // Non-blocking: hand off to the app's background create (spec D2) so
          // the user keeps navigating while it runs.
          if (nm) startCreate(nm);
        } else setOverlay({ ...overlay, text: editText(overlay.text, key) });
        return;
      case "adopt":
        if (isCancel) close();
        else if (isEnter) {
          const br = overlay.text.trim();
          if (br)
            runMutationRouted("Adopt " + br, "adopt", [br], () => adoptBranch(br));
          else close();
        } else setOverlay({ ...overlay, text: editText(overlay.text, key) });
        return;
      case "group":
        if (isCancel) close();
        else if (isEnter) {
          const nm = overlay.text.trim();
          if (nm) {
            const done = overlay.onDone;
            runMutation("Group → " + nm, () => groupSlices(nm, overlay.slices));
            done();
          } else close();
        } else setOverlay({ ...overlay, text: editText(overlay.text, key) });
        return;
      case "candidates": {
        const { items, sel } = overlay;
        if (name === "j" || name === "down")
          setOverlay({ ...overlay, sel: Math.min(items.length - 1, sel + 1) });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, sel: Math.max(0, sel - 1) });
        else if (isCancel || name === "q") close();
        else if (items.length > 0) {
          const c = items[sel]!;
          if (name === "i" || isEnter)
            runMutation("Import " + c.slice, () => importCandidate(c.path));
          else if (name === "a")
            runMutationRouted("Adopt " + c.branch, "adopt", [c.branch], () =>
              adoptBranch(c.branch),
            );
          else if (name === "x") runMutation("Ignore " + c.slice, () => ignoreCandidate(c.path));
        }
        return;
      }
      case "editorPicker": {
        const { editors: eds, sel } = overlay;
        if (name === "j" || name === "down")
          setOverlay({ ...overlay, sel: Math.min(eds.length - 1, sel + 1) });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, sel: Math.max(0, sel - 1) });
        else if (isCancel || name === "q") close();
        else if (isEnter && eds[sel])
          runEdit(
            overlay.slice,
            overlay.repo,
            overlay.path,
            overlay.line,
            eds[sel]!.bin,
          );
        return;
      }
      case "agentPicker": {
        const { agents, sel, onPick } = overlay;
        const quick = quickPickIndex(name, agents.length);
        if (quick !== null) {
          close();
          onPick(agents[quick]!);
        } else if (name === "j" || name === "down")
          setOverlay({ ...overlay, sel: Math.min(agents.length - 1, sel + 1) });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, sel: Math.max(0, sel - 1) });
        else if (isCancel || name === "q") close();
        else if (isEnter && agents[sel]) {
          close();
          onPick(agents[sel]!);
        }
        return;
      }
      case "conflicts":
        if (name === "j" || name === "down")
          setOverlay({ ...overlay, scroll: overlay.scroll + 1 });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, scroll: Math.max(0, overlay.scroll - 1) });
        else if (name === "!" || isCancel || name === "q") close();
        return;
      case "summary":
        if (name === "j" || name === "down")
          setOverlay({ ...overlay, scroll: overlay.scroll + 1 });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, scroll: Math.max(0, overlay.scroll - 1) });
        else if (name === "S") openSummary(overlay.slice, true);
        else if (name === "s" || isCancel || name === "q") close();
        return;
      case "comment":
        if (isCancel) close();
        else if (isEnter) {
          const body = overlay.text.trim();
          if (!body) {
            close();
            return;
          }
          const { ctx, onAdded } = overlay;
          setOverlay({ kind: "working", text: "Adding comment…" });
          reviewAdd({
            slice: ctx.slice,
            repo: ctx.repo,
            branch: ctx.branch,
            file: ctx.file,
            line: ctx.line,
            endLine: ctx.endLine,
            side: ctx.side,
            hunk: ctx.hunk,
            body,
          }).then(
            (res) => {
              if (res.code === 0) {
                toast("Added review comment", "ci-pass");
                onAdded();
                close();
              } else {
                setOverlay({
                  kind: "result",
                  title: "Add comment — failed",
                  body: (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
                  status: "failure",
                });
              }
            },
            (err) =>
              setOverlay({
                kind: "result",
                title: "Add comment — failed",
                body: String(err),
                status: "failure",
              }),
          );
        } else setOverlay({ ...overlay, text: editText(overlay.text, key) });
        return;
      case "review": {
        const { slice, comments, sel, confirmSend, onChanged } = overlay;
        if (confirmSend) {
          if (name === "y" || isEnter) {
            const n = comments?.length ?? 0;
            setOverlay({ kind: "working", text: "Sending review…" });
            reviewSend(slice).then(
              (res) => {
                if (res.code === 0) {
                  toast(`Sent ${n} comment${n === 1 ? "" : "s"} to ${slice}`, "ci-pass");
                  onChanged();
                  close();
                } else {
                  // No running session (or other refusal) — surface the CLI's
                  // guidance verbatim as a neutral warn, not a red failure.
                  setOverlay({
                    kind: "result",
                    title: "Send review",
                    body:
                      (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
                    status: "warn",
                  });
                }
              },
              (err) =>
                setOverlay({
                  kind: "result",
                  title: "Send review — failed",
                  body: String(err),
                  status: "failure",
                }),
            );
          } else if (name === "n" || isCancel) setOverlay({ ...overlay, confirmSend: false });
          return;
        }
        const list = comments ?? [];
        if (name === "?") {
          helpReturn.current = overlay;
          setOverlay({ kind: "help" });
        } else if (name === "j" || name === "down")
          setOverlay({ ...overlay, sel: clampReviewSel(sel + 1, list.length) });
        else if (name === "k" || name === "up")
          setOverlay({ ...overlay, sel: clampReviewSel(sel - 1, list.length) });
        else if (name === "x" && list[sel]) {
          const id = list[sel]!.id;
          reviewRm(slice, id).then(
            (res) => {
              if (res.code === 0) {
                onChanged();
                reloadReview(slice);
              }
            },
            () => {},
          );
        } else if (name === "s" && list.length > 0) setOverlay({ ...overlay, confirmSend: true });
        else if (isCancel || name === "q") close();
        return;
      }
    }
  });

  return api;
}

function renderOverlay(
  overlay: Overlay,
  conflicts: ConflictsResult | null,
  view: "browser" | "cockpit",
  height: number,
): ReactNode {
  if (!overlay) return null;
  switch (overlay.kind) {
    case "help":
      return <Help view={view} />;
    case "swap":
      return <SwapOverlay slice={overlay.slice} active={overlay.active} dirty={overlay.dirty} />;
    case "stack":
      return <StackActionsOverlay slices={overlay.slices} conflictWith={overlay.conflictWith} />;
    case "remove":
      return <RemoveOverlay slices={overlay.slices} />;
    case "ciRerun":
      return <CiRerunOverlay slice={overlay.slice} />;
    case "create":
      return <CreateOverlay text={overlay.text} />;
    case "adopt":
      return <AdoptOverlay text={overlay.text} />;
    case "group":
      return <GroupOverlay slices={overlay.slices} text={overlay.text} />;
    case "candidates":
      return <CandidatesOverlay items={overlay.items} sel={overlay.sel} />;
    case "editorPicker":
      return (
        <EditorPickerOverlay
          editors={overlay.editors}
          sel={overlay.sel}
          slice={overlay.slice}
          repo={overlay.repo}
          path={overlay.path}
          line={overlay.line}
        />
      );
    case "agentPicker":
      return <AgentPickerOverlay agents={overlay.agents} sel={overlay.sel} slice={overlay.slice} />;
    case "conflicts":
      return <ConflictRadarOverlay conflicts={conflicts} scroll={overlay.scroll} height={height} />;
    case "summary":
      return (
        <SummaryOverlay
          slice={overlay.slice}
          ai={overlay.ai}
          loading={overlay.loading}
          text={overlay.text}
          scroll={overlay.scroll}
          height={height}
        />
      );
    case "comment":
      return <CommentComposerOverlay ctx={overlay.ctx} text={overlay.text} />;
    case "review":
      return (
        <ReviewListOverlay
          slice={overlay.slice}
          comments={overlay.comments}
          sel={overlay.sel}
          confirmSend={overlay.confirmSend}
          height={height}
        />
      );
    case "working":
      return <WorkingOverlay text={overlay.text} />;
    case "result":
      return <ResultOverlay title={overlay.title} body={overlay.body} status={overlay.status} />;
  }
  return null;
}

// useOverlays owns the single active overlay, its keyboard routing, and the
// shared one-shot mutation runner (working → result, then refresh). app.go runs
// a priority chain over many independent overlay flags; here a single nullable
// discriminated union makes the priority implicit — whichever overlay is set is
// the one that handles keys and renders. Browser / cockpit stay disabled while
// `active` is true, so exactly one keyboard owner is live at a time.

import { useCallback, useState, type ReactNode } from "react";
import { useKeyboard } from "@opentui/react";
import type { KeyEvent } from "@opentui/core";
import type { Candidate, ConflictsResult } from "../rpc/types";
import {
  activate,
  adoptBranch,
  copyToClipboard,
  createSlice,
  deactivate,
  groupSlices,
  ignoreCandidate,
  importCandidate,
  mergeSlice,
  openUrl,
  prStackMarkdown,
  removeSlice,
  restackSlice,
  submitSlice,
  summarySlice,
  syncSlice,
  ungroupSlice,
  type MutateResult,
} from "../rpc/mutate";
import { editText } from "./textinput";
import {
  CandidatesOverlay,
  ConflictRadarOverlay,
  CreateOverlay,
  GroupOverlay,
  RemoveOverlay,
  ResultOverlay,
  StackActionsOverlay,
  SummaryOverlay,
  SwapOverlay,
  WorkingOverlay,
} from "./overlays";
import { Help } from "../components/help";

type Overlay =
  | { kind: "help" }
  | { kind: "swap"; slice: string; active: boolean }
  | { kind: "stack"; slices: string[]; conflictWith: string[] }
  | { kind: "remove"; slices: string[] }
  | { kind: "create"; text: string }
  | { kind: "group"; slices: string[]; text: string; onDone: () => void }
  | { kind: "candidates"; items: Candidate[]; sel: number }
  | { kind: "conflicts"; scroll: number }
  | { kind: "summary"; slice: string; ai: boolean; loading: boolean; text: string; scroll: number }
  | { kind: "working"; text: string }
  | { kind: "result"; title: string; body: string; ok: boolean }
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
  create(): void;
  candidates(items: Candidate[]): void;
  group(slices: string[], onDone: () => void): void;
  conflictRadar(): void;
  summary(slice: string, ai: boolean): void;
  info(title: string, body: string): void;
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
  const { refresh, conflicts, view, height } = args;
  const [overlay, setOverlay] = useState<Overlay>(null);
  const close = useCallback(() => setOverlay(null), []);

  const runMutation = useCallback(
    (title: string, fn: () => Promise<MutateResult>) => {
      setOverlay({ kind: "working", text: title + "…" });
      fn().then(
        (res) => {
          const ok = res.code === 0;
          setOverlay({
            kind: "result",
            title: ok ? title : title + " — failed",
            body: (res.stdout + (res.stderr ? "\n" + res.stderr : "")).trim() || "(no output)",
            ok,
          });
          refresh();
        },
        (err) =>
          setOverlay({ kind: "result", title: title + " — failed", body: String(err), ok: false }),
      );
    },
    [refresh],
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

  const api: OverlayApi = {
    active: overlay !== null,
    view,
    node: renderOverlay(overlay, conflicts, view, height),
    help: () => setOverlay({ kind: "help" }),
    swap: (slice, active) => setOverlay({ kind: "swap", slice, active }),
    stack: (slices, conflictWith) => setOverlay({ kind: "stack", slices, conflictWith }),
    remove: (slices) => setOverlay({ kind: "remove", slices }),
    create: () => setOverlay({ kind: "create", text: "" }),
    candidates: (items) => setOverlay({ kind: "candidates", items, sel: 0 }),
    group: (slices, onDone) => setOverlay({ kind: "group", slices, text: "", onDone }),
    conflictRadar: () => setOverlay({ kind: "conflicts", scroll: 0 }),
    summary: openSummary,
    info: (title, body) => setOverlay({ kind: "result", title, body, ok: true }),
    ungroup: (slice) => runMutation("Ungroup " + slice, () => ungroupSlice(slice)),
    yankDiff: (text) => runMutation("Yank diff", () => copyToClipboard(text)),
    yankPrStack: (slice) =>
      runMutation("Yank PR stack", async () => {
        const md = await prStackMarkdown(slice);
        if (md.code !== 0) return md;
        const clip = await copyToClipboard(md.stdout || "(no PRs)");
        return {
          code: clip.code,
          stdout: (md.stdout || "(no PRs)") + "\n\n" + clip.stdout,
          stderr: clip.stderr,
        };
      }),
    openPr: (url) => runMutation("Open PR", () => openUrl(url)),
  };

  useKeyboard((key: KeyEvent) => {
    if (!overlay) return;
    const name = key.name;
    const isEnter = name === "return" || name === "enter";
    const isCancel = name === "escape";

    switch (overlay.kind) {
      case "help":
        if (name === "?" || isCancel || name === "q") close();
        return;
      case "working":
        return;
      case "result":
        if (isEnter || isCancel || name === "q") close();
        return;
      case "swap":
        if (name === "y" || isEnter) runMutation(overlay.active ? "Swap out" : "Swap in", () =>
          overlay.active ? deactivate(overlay.slice) : activate(overlay.slice),
        );
        else if (name === "n" || isCancel) close();
        return;
      case "stack": {
        const first = overlay.slices[0] ?? "";
        if (name === "r")
          runMutation("Restack " + overlay.slices.join(", "), () =>
            runSequential(overlay.slices, restackSlice),
          );
        else if (name === "p") runMutation("Submit " + first, () => submitSlice(first));
        else if (name === "m") runMutation("Merge " + first, () => mergeSlice(first));
        else if (name === "s") runMutation("Sync " + first, () => syncSlice(first));
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
      case "create":
        if (isCancel) close();
        else if (isEnter) {
          const nm = overlay.text.trim();
          if (nm) runMutation("Create " + nm, () => createSlice(nm));
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
          else if (name === "a") runMutation("Adopt " + c.branch, () => adoptBranch(c.branch));
          else if (name === "x") runMutation("Ignore " + c.slice, () => ignoreCandidate(c.path));
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
      return <SwapOverlay slice={overlay.slice} active={overlay.active} />;
    case "stack":
      return <StackActionsOverlay slices={overlay.slices} conflictWith={overlay.conflictWith} />;
    case "remove":
      return <RemoveOverlay slices={overlay.slices} />;
    case "create":
      return <CreateOverlay text={overlay.text} />;
    case "group":
      return <GroupOverlay slices={overlay.slices} text={overlay.text} />;
    case "candidates":
      return <CandidatesOverlay items={overlay.items} sel={overlay.sel} />;
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
    case "working":
      return <WorkingOverlay text={overlay.text} />;
    case "result":
      return <ResultOverlay title={overlay.title} body={overlay.body} ok={overlay.ok} />;
  }
  return null;
}

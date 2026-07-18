// Presentational overlay cards. Each takes plain data + the current selection /
// input / scroll state and renders; all key handling and side effects live in
// useOverlays. Styling mirrors the Bubble Tea overlays (candidatepane.go,
// conflictpane.go, the swap / stack / remove prompts in app.go).

import { useEffect, useState, type ReactNode } from "react";
import type { Candidate, ConflictsResult } from "../rpc/types";
import type { EditorSpec } from "../editor/detect";
import { color, glyph } from "../theme";
import { Overlay } from "../components/overlay";
import { BOLD, DIM } from "../components/ui";
import { stripSgr } from "../util/ansi";

function KeyHint({ k, label }: { k: string; label: string }): ReactNode {
  return (
    <>
      <span fg={color.candidate} attributes={BOLD}>
        [{k}]
      </span>
      <span fg={color.fg}> {label}   </span>
    </>
  );
}

export function SwapOverlay({
  slice,
  active,
  dirty,
}: {
  slice: string;
  active: boolean;
  dirty: boolean;
}): ReactNode {
  const detail = active
    ? "Restores each primary to its previous branch."
    : "Puts each primary on slis/live/" + slice + " at the slice tip.";
  return (
    <Overlay title={`Swap — ${slice}`} width={58}>
      <text wrapMode="none">
        <span fg={color.fg}>{active ? "swap OUT " : "swap IN "}</span>
        <span fg={color.title} attributes={BOLD}>
          {slice}
        </span>
        <span fg={color.fg}>?</span>
      </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        {detail}
      </text>
      {dirty && !active ? (
        <text fg={color.wait} wrapMode="none">
          ⚠ a primary has uncommitted work — [s] stashes it, popped back on swap-out.
        </text>
      ) : null}
      <text> </text>
      <text wrapMode="none">
        <KeyHint k="y" label="confirm" />
        {dirty && !active ? <KeyHint k="s" label="stash + swap in" /> : null}
        <KeyHint k="n/esc" label="cancel" />
      </text>
    </Overlay>
  );
}

export function EditorPickerOverlay({
  editors,
  sel,
  slice,
  repo,
}: {
  editors: EditorSpec[];
  sel: number;
  slice: string;
  repo?: string;
}): ReactNode {
  const target = repo ? `${slice} · ${repo}` : slice;
  return (
    <Overlay title="Open in which editor?" width={58}>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        {target}
      </text>
      <text> </text>
      {editors.map((e, i) => {
        const focused = i === sel;
        return (
          <text key={e.bin} wrapMode="none">
            <span fg={color.cursorBar}>{focused ? glyph.focusBar + " " : "  "}</span>
            <span fg={focused ? color.white : color.fg} attributes={focused ? BOLD : 0}>
              {e.name}
            </span>
            <span fg={color.dim}>{"  (" + e.bin + ")"}</span>
          </text>
        );
      })}
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        ↑/↓ select · enter open (remembered) · esc cancel
      </text>
    </Overlay>
  );
}

export function WorkingOverlay({ text }: { text: string }): ReactNode {
  const [frame, setFrame] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setFrame((f) => (f + 1) % SPINNER.length), 90);
    return () => clearInterval(id);
  }, []);
  return (
    <Overlay title="Working" width={52}>
      <text wrapMode="none">
        <span fg={color.title} attributes={BOLD}>
          {SPINNER[frame]}{" "}
        </span>
        <span fg={color.fg}>{text}</span>
      </text>
    </Overlay>
  );
}
const SPINNER = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];

export function ResultOverlay({
  title,
  body,
  ok,
}: {
  title: string;
  body: string;
  ok: boolean;
}): ReactNode {
  const lines = body.split("\n").slice(0, 16);
  return (
    <Overlay title={title} width={78}>
      {lines.map((l, i) => (
        <text key={i} fg={ok ? color.fg : color.missing} wrapMode="none">
          {l === "" ? " " : stripSgr(l)}
        </text>
      ))}
      <text> </text>
      <text fg={color.dim} attributes={DIM}>
        press enter / esc to close
      </text>
    </Overlay>
  );
}

export function StackActionsOverlay({
  slices,
  conflictWith,
}: {
  slices: string[];
  conflictWith: string[];
}): ReactNode {
  const target = slices[0] ?? "";
  return (
    <Overlay title="Stack actions" width={64}>
      <text wrapMode="none">
        <span fg={color.fg}>target: </span>
        <span fg={color.title} attributes={BOLD}>
          {slices.join(", ")}
        </span>
      </text>
      {conflictWith.length > 0 ? (
        <text fg={color.wait} wrapMode="none">
          ⚠ {target} shares changed files with: {conflictWith.join(", ")}{" "}
          (may be stale; committed changes only)
        </text>
      ) : null}
      <text> </text>
      <text wrapMode="none">
        <KeyHint k="r" label="restack" />
        <KeyHint k="p" label="submit" />
        <KeyHint k="m" label="merge" />
        <KeyHint k="s" label="sync" />
      </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        restack runs across all targets; submit / merge / sync act on the first
        target. [n/esc] cancel
      </text>
    </Overlay>
  );
}

export function RemoveOverlay({ slices }: { slices: string[] }): ReactNode {
  return (
    <Overlay title="Clear finished slice(s)" width={64}>
      <text wrapMode="none">
        <span fg={color.fg}>remove </span>
        <span fg={color.title} attributes={BOLD}>
          {slices.join(", ")}
        </span>
        <span fg={color.fg}>?</span>
      </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        Deletes worktrees + merged branches, kills the tmux session.
      </text>
      <text> </text>
      <text wrapMode="none">
        <KeyHint k="y" label="remove" />
        <KeyHint k="f" label="force (dirty + unmerged)" />
        <KeyHint k="n/esc" label="cancel" />
      </text>
    </Overlay>
  );
}

function TextInputRow({ text }: { text: string }): ReactNode {
  return (
    <text wrapMode="none">
      <span fg={color.white}>{text}</span>
      <span fg={color.cursorBar} attributes={BOLD}>
        ▎
      </span>
    </text>
  );
}

export function CreateOverlay({ text }: { text: string }): ReactNode {
  return (
    <Overlay title="Create slice" width={58}>
      <text fg={color.candidate} attributes={BOLD} wrapMode="none">
        ✎ new slice name
      </text>
      <TextInputRow text={text} />
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        Creates a worktree per repo (off each trunk). [enter] create · [esc]
        cancel
      </text>
    </Overlay>
  );
}

export function GroupOverlay({
  slices,
  text,
}: {
  slices: string[];
  text: string;
}): ReactNode {
  return (
    <Overlay title="Group slices" width={60}>
      <text wrapMode="none">
        <span fg={color.fg}>grouping: </span>
        <span fg={color.title} attributes={BOLD}>
          {slices.join(", ")}
        </span>
      </text>
      <text> </text>
      <text fg={color.candidate} attributes={BOLD} wrapMode="none">
        group name
      </text>
      <TextInputRow text={text} />
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        [enter] group · [esc] cancel
      </text>
    </Overlay>
  );
}

export function CandidatesOverlay({
  items,
  sel,
}: {
  items: Candidate[];
  sel: number;
}): ReactNode {
  return (
    <Overlay title="New worktrees — import to manage as slices" width={78}>
      {items.length === 0 ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          No new worktrees — everything is managed or ignored.
        </text>
      ) : (
        items.map((c, i) => {
          const focused = i === sel;
          const dir = c.path.replace(/\/[^/]*$/, "");
          return (
            <text key={c.path} wrapMode="none">
              <span fg={color.cursorBar}>{focused ? glyph.focusBar + " " : "  "}</span>
              <span fg={focused ? color.white : color.fg} attributes={focused ? BOLD : 0}>
                {c.slice}
              </span>
              <span fg={color.dim}>
                {"  "}
                {c.repo} · {c.branch}  {dir}
              </span>
            </text>
          );
        })
      )}
      <text> </text>
      <text wrapMode="none">
        <KeyHint k="i" label="import" />
        <KeyHint k="a" label="adopt branch" />
        <KeyHint k="x" label="ignore" />
        <KeyHint k="j/k" label="move" />
        <KeyHint k="esc" label="close" />
      </text>
    </Overlay>
  );
}

export function ConflictRadarOverlay({
  conflicts,
  scroll,
  height,
}: {
  conflicts: ConflictsResult | null;
  scroll: number;
  height: number;
}): ReactNode {
  const overlaps = conflicts?.overlaps ?? [];
  const incomplete = conflicts?.incomplete ?? [];
  const maxRows = Math.max(5, height - 12);
  const start = Math.min(Math.max(0, scroll), Math.max(0, overlaps.length - maxRows));
  const shown = overlaps.slice(start, start + maxRows);
  return (
    <Overlay title="Conflict radar — files changed by >1 slice" width={82}>
      {overlaps.length === 0 && incomplete.length === 0 ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          No cross-slice conflicts — no file is changed by more than one slice.
        </text>
      ) : null}
      {start > 0 ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          ↑ {start} more above
        </text>
      ) : null}
      {shown.map((o) => (
        <text key={o.repo + o.path} wrapMode="none">
          <span fg={color.repoHeader} attributes={BOLD}>
            {o.repo}
          </span>
          <span fg={color.fg}>  {o.path}</span>
          <span fg={color.dim}>  ← {o.slices.join(", ")}</span>
        </text>
      ))}
      {start + maxRows < overlaps.length ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          ↓ {overlaps.length - start - maxRows} more below
        </text>
      ) : null}
      {incomplete.length > 0 ? (
        <text fg={color.wait} wrapMode="none">
          radar incomplete (diff unavailable) for: {incomplete.join(", ")}
        </text>
      ) : null}
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        File overlap is a heads-up, not a guaranteed merge conflict. Committed
        changes only. j/k scroll · ! / esc close
      </text>
    </Overlay>
  );
}

export function SummaryOverlay({
  slice,
  ai,
  loading,
  text,
  scroll,
  height,
}: {
  slice: string;
  ai: boolean;
  loading: boolean;
  text: string;
  scroll: number;
  height: number;
}): ReactNode {
  const lines = text.split("\n").map(stripSgr);
  const maxRows = Math.max(5, height - 10);
  const start = Math.min(Math.max(0, scroll), Math.max(0, lines.length - maxRows));
  const shown = lines.slice(start, start + maxRows);
  return (
    <Overlay title={`${ai ? "AI summary" : "Summary"} — ${slice}`} width={86}>
      {loading ? (
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          {ai ? "asking claude…" : "reading commit log…"}
        </text>
      ) : (
        <>
          {start > 0 ? (
            <text fg={color.dim} attributes={DIM} wrapMode="none">
              ↑ {start} more above
            </text>
          ) : null}
          {shown.map((l, i) => (
            <text key={i} fg={color.fg} wrapMode="none">
              {l === "" ? " " : l}
            </text>
          ))}
          {start + maxRows < lines.length ? (
            <text fg={color.dim} attributes={DIM} wrapMode="none">
              ↓ {lines.length - start - maxRows} more below
            </text>
          ) : null}
        </>
      )}
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        j/k scroll · s summary · S force AI · esc close
      </text>
    </Overlay>
  );
}

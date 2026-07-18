// Presentational overlay cards. Each takes plain data + the current selection /
// input / scroll state and renders; all key handling and side effects live in
// useOverlays. Styling mirrors the Bubble Tea overlays (candidatepane.go,
// conflictpane.go, the swap / stack / remove prompts in app.go).

import type { ReactNode } from "react";
import type { AgentSpec, Candidate, ConflictsResult, ReviewComment } from "../rpc/types";
import type { CommentContext } from "../review/context";
import { agentCmdline } from "../term/agentpick";
import type { EditorSpec } from "../editor/detect";
import { glyph, theme, type ResultStatus } from "../theme";
import { Card } from "../components/card";
import { Spinner } from "../components/spinner";
import { BOLD } from "../components/ui";
import { stripSgr } from "../util/ansi";

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
    <Card
      title={`Swap · ${slice}`}
      width={58}
      hints={[
        { key: "y", label: "confirm" },
        ...(dirty && !active ? [{ key: "s", label: "stash + swap" }] : []),
        { key: "esc", label: "cancel" },
      ]}
    >
      <text wrapMode="none">
        <span fg={theme.text}>{active ? "swap OUT " : "swap IN "}</span>
        <span fg={theme.textBright} attributes={BOLD}>
          {slice}
        </span>
        <span fg={theme.text}>?</span>
      </text>
      <text fg={theme.textDim} wrapMode="none">
        {detail}
      </text>
      {dirty && !active ? (
        <text fg={theme.attn} wrapMode="none">
          {glyph.dirty} a primary has uncommitted work — [s] stashes it, popped back on swap-out.
        </text>
      ) : null}
    </Card>
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
    <Card
      title="Open in which editor?"
      subtitle={target}
      width={58}
      hints={[
        { key: "↑/↓", label: "select" },
        { key: "enter", label: "open" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {editors.map((e, i) => {
        const focused = i === sel;
        return (
          <text key={e.bin} wrapMode="none">
            <span fg={theme.focus}>{focused ? glyph.focusBar + " " : "  "}</span>
            <span fg={focused ? theme.textBright : theme.textDim} attributes={focused ? BOLD : 0}>
              {e.name}
            </span>
            <span fg={theme.textFaint}>{"  (" + e.bin + ")"}</span>
          </text>
        );
      })}
    </Card>
  );
}

export function AgentPickerOverlay({
  agents,
  sel,
  slice,
}: {
  agents: AgentSpec[];
  sel: number;
  slice: string;
}): ReactNode {
  return (
    <Card
      title="Launch which agent?"
      subtitle={slice}
      width={58}
      hints={[
        { key: "1-9", label: "quick pick" },
        { key: "↑/↓", label: "select" },
        { key: "enter", label: "launch" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {agents.map((a, i) => {
        const focused = i === sel;
        return (
          <text key={a.name} wrapMode="none">
            <span fg={theme.focus}>{focused ? glyph.focusBar + " " : "  "}</span>
            <span fg={theme.textFaint}>{(i < 9 ? String(i + 1) : " ") + " "}</span>
            <span fg={focused ? theme.textBright : theme.textDim} attributes={focused ? BOLD : 0}>
              {a.name}
            </span>
            <span fg={theme.textFaint}>{"  (" + agentCmdline(a.cmd) + ")"}</span>
          </text>
        );
      })}
    </Card>
  );
}

export function WorkingOverlay({ text }: { text: string }): ReactNode {
  return (
    <Card title="Working" width={52}>
      <text wrapMode="none">
        <Spinner />
        <span fg={theme.text}> {text}</span>
      </text>
    </Card>
  );
}

export function ResultOverlay({
  title,
  body,
  status,
}: {
  title: string;
  body: string;
  status: ResultStatus;
}): ReactNode {
  const lines = body.split("\n").slice(0, 16);
  return (
    <Card
      title={title}
      status={status}
      width={78}
      hints={[
        { key: "enter", label: "close" },
        { key: "esc", label: "close" },
      ]}
    >
      {lines.map((l, i) => (
        <text key={i} wrapMode="none" fg={theme.text}>
          {l === "" ? " " : stripSgr(l)}
        </text>
      ))}
    </Card>
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
    <Card
      title="Stack actions"
      subtitle={`target: ${slices.join(", ")}`}
      width={64}
      hints={[
        { key: "r", label: "restack" },
        { key: "p", label: "submit" },
        { key: "m", label: "merge" },
        { key: "s", label: "sync" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {conflictWith.length > 0 ? (
        <text fg={theme.attn} wrapMode="word">
          {glyph.overlap} {target} shares changed files with: {conflictWith.join(", ")} (may be
          stale; committed changes only)
        </text>
      ) : null}
      <text fg={theme.textDim} wrapMode="word">
        restack runs across all targets; submit / merge / sync act on the first target.
      </text>
    </Card>
  );
}

export function CiRerunOverlay({ slice }: { slice: string }): ReactNode {
  return (
    <Card
      title={`Re-run failing CI · ${slice}`}
      width={62}
      hints={[
        { key: "y", label: "re-run" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <text wrapMode="none">
        <span fg={theme.text}>re-trigger failed CI runs for </span>
        <span fg={theme.textBright} attributes={BOLD}>
          {slice}
        </span>
        <span fg={theme.text}>?</span>
      </text>
      <text fg={theme.textDim} wrapMode="none">
        Runs `gh run rerun --failed` for each repo's PR (a CI write).
      </text>
    </Card>
  );
}

export function RemoveOverlay({ slices }: { slices: string[] }): ReactNode {
  return (
    <Card
      title="Clear finished slice(s)"
      width={64}
      hints={[
        { key: "y", label: "remove" },
        { key: "f", label: "force" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <text wrapMode="none">
        <span fg={theme.text}>remove </span>
        <span fg={theme.textBright} attributes={BOLD}>
          {slices.join(", ")}
        </span>
        <span fg={theme.text}>?</span>
      </text>
      <text fg={theme.textDim} wrapMode="none">
        Deletes worktrees + merged branches, kills the tmux session.
      </text>
    </Card>
  );
}

function TextInputRow({ text }: { text: string }): ReactNode {
  return (
    <text wrapMode="none">
      <span fg={theme.textBright}>{text}</span>
      <span fg={theme.focus} attributes={BOLD}>
        {glyph.focusBar}
      </span>
    </text>
  );
}

export function CreateOverlay({ text }: { text: string }): ReactNode {
  return (
    <Card
      title="Create slice"
      width={58}
      hints={[
        { key: "enter", label: "create" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <text fg={theme.focus} attributes={BOLD} wrapMode="none">
        new slice name
      </text>
      <TextInputRow text={text} />
      <text fg={theme.textDim} wrapMode="none">
        Creates a worktree per repo (off each trunk).
      </text>
    </Card>
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
    <Card
      title="Group slices"
      subtitle={`grouping: ${slices.join(", ")}`}
      width={60}
      hints={[
        { key: "enter", label: "group" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <text fg={theme.focus} attributes={BOLD} wrapMode="none">
        group name
      </text>
      <TextInputRow text={text} />
    </Card>
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
    <Card
      title="New worktrees — import to manage as slices"
      width={78}
      hints={[
        { key: "i", label: "import" },
        { key: "a", label: "adopt branch" },
        { key: "x", label: "ignore" },
        { key: "j/k", label: "move" },
        { key: "esc", label: "close" },
      ]}
    >
      {items.length === 0 ? (
        <text fg={theme.textDim} wrapMode="none">
          No new worktrees — everything is managed or ignored.
        </text>
      ) : (
        items.map((c, i) => {
          const focused = i === sel;
          const dir = c.path.replace(/\/[^/]*$/, "");
          return (
            <text key={c.path} wrapMode="none">
              <span fg={theme.focus}>{focused ? glyph.focusBar + " " : "  "}</span>
              <span fg={focused ? theme.textBright : theme.text} attributes={focused ? BOLD : 0}>
                {c.slice}
              </span>
              <span fg={theme.textDim}>
                {"  "}
                {c.repo} · {c.branch}
              </span>
              <span fg={theme.textFaint}>  {dir}</span>
            </text>
          );
        })
      )}
    </Card>
  );
}

// ── inline review (F2) ────────────────────────────────────────────────────────

// The comment composer: captured context (repo · branch · file:line) + a short
// excerpt for reference, and a single-line body input. Submitting persists via
// `slis review add`. Body is single-line in v1 (the shared editText reducer has
// no soft-wrap); enter submits, esc cancels.
export function CommentComposerOverlay({
  ctx,
  text,
}: {
  ctx: CommentContext;
  text: string;
}): ReactNode {
  const excerpt = ctx.hunk ? ctx.hunk.split("\n").slice(0, 6) : [];
  return (
    <Card
      title="Comment on this line"
      subtitle={`${ctx.repo} · ${ctx.branch || "?"} ${glyph.arrow} ${ctx.file}:${ctx.line}`}
      width={76}
      hints={[
        { key: "enter", label: "add comment" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {excerpt.length > 0 ? (
        <box flexDirection="column" marginBottom={1}>
          {excerpt.map((l, i) => (
            <text key={i} fg={theme.textFaint} wrapMode="none">
              {l === "" ? " " : stripSgr(l)}
            </text>
          ))}
        </box>
      ) : null}
      <text fg={theme.focus} attributes={BOLD} wrapMode="none">
        instruction for the agent
      </text>
      <TextInputRow text={text} />
      <text fg={theme.textDim} wrapMode="none">
        Delivered with `C` {glyph.arrow} `s`, or `slis review send {ctx.slice}`.
      </text>
    </Card>
  );
}

// The pending-review overlay: lists the slice's queued comments (j/k, x delete),
// with `s` to send the batch to the agent. `confirmSend` swaps the list for a
// send-confirmation card. A windowed list keeps the selection visible without a
// nested scrollbox (matching the other list overlays).
export function ReviewListOverlay({
  slice,
  comments,
  sel,
  confirmSend,
  height,
}: {
  slice: string;
  comments: ReviewComment[] | null;
  sel: number;
  confirmSend: boolean;
  height: number;
}): ReactNode {
  const list = comments ?? [];

  if (confirmSend) {
    return (
      <Card
        title={`Send review · ${slice}`}
        width={64}
        hints={[
          { key: "y", label: "send" },
          { key: "esc", label: "cancel" },
        ]}
      >
        <text wrapMode="word">
          <span fg={theme.text}>Send </span>
          <span fg={theme.textBright} attributes={BOLD}>
            {list.length} comment{list.length === 1 ? "" : "s"}
          </span>
          <span fg={theme.text}> to {slice}'s agent session?</span>
        </text>
        <text fg={theme.textDim} wrapMode="word">
          Injects one structured prompt into the running tmux session, then clears the batch.
        </text>
      </Card>
    );
  }

  const count = list.length;
  const subtitle = comments === null ? "loading…" : `${count} pending`;
  const maxRows = Math.max(4, Math.floor((height - 12) / 2));
  const start = Math.min(Math.max(0, sel - maxRows + 1), Math.max(0, count - maxRows));
  const shown = list.slice(start, start + maxRows);

  return (
    <Card
      title={`Review ${glyph.comment} ${slice}`}
      subtitle={subtitle}
      width={82}
      hints={[
        { key: "j/k", label: "move" },
        { key: "x", label: "delete" },
        ...(count > 0 ? [{ key: "s", label: "send to agent" }] : []),
        { key: "esc", label: "close" },
      ]}
    >
      {comments === null ? (
        <text fg={theme.textDim} wrapMode="none">
          reading pending comments…
        </text>
      ) : count === 0 ? (
        <text fg={theme.textDim} wrapMode="none">
          No pending comments — press `c` on a diff or file line to add one.
        </text>
      ) : (
        <>
          {start > 0 ? (
            <text fg={theme.textFaint} wrapMode="none">
              ↑ {start} more above
            </text>
          ) : null}
          {shown.map((c, i) => {
            const idx = start + i;
            const focused = idx === sel;
            return (
              <box key={c.id} flexDirection="column">
                <text wrapMode="none">
                  <span fg={theme.focus}>{focused ? glyph.focusBar + " " : "  "}</span>
                  <span fg={theme.textDim}>{c.repo} </span>
                  <span fg={focused ? theme.textBright : theme.text}>
                    {c.file}:{c.line}
                  </span>
                </text>
                <text wrapMode="none">
                  <span fg={theme.textFaint}>    </span>
                  <span fg={focused ? theme.text : theme.textDim}>{truncate(c.body, 72)}</span>
                </text>
              </box>
            );
          })}
          {start + maxRows < count ? (
            <text fg={theme.textFaint} wrapMode="none">
              ↓ {count - start - maxRows} more below
            </text>
          ) : null}
        </>
      )}
    </Card>
  );
}

function truncate(s: string, n: number): string {
  const flat = s.replace(/\s+/g, " ").trim();
  return flat.length > n ? flat.slice(0, n - 1) + "…" : flat;
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
    <Card
      title="Conflict radar — files changed by >1 slice"
      width={82}
      hints={[
        { key: "j/k", label: "scroll" },
        { key: "!", label: "close" },
        { key: "esc", label: "close" },
      ]}
    >
      {overlaps.length === 0 && incomplete.length === 0 ? (
        <text fg={theme.textDim} wrapMode="none">
          No cross-slice conflicts — no file is changed by more than one slice.
        </text>
      ) : null}
      {start > 0 ? (
        <text fg={theme.textFaint} wrapMode="none">
          ↑ {start} more above
        </text>
      ) : null}
      {shown.map((o) => (
        <text key={o.repo + o.path} wrapMode="none">
          <span fg={theme.focus} attributes={BOLD}>
            {o.repo}
          </span>
          <span fg={theme.text}>  {o.path}</span>
          <span fg={theme.textDim}>  ← {o.slices.join(", ")}</span>
        </text>
      ))}
      {start + maxRows < overlaps.length ? (
        <text fg={theme.textFaint} wrapMode="none">
          ↓ {overlaps.length - start - maxRows} more below
        </text>
      ) : null}
      {incomplete.length > 0 ? (
        <text fg={theme.attn} wrapMode="word">
          radar incomplete (diff unavailable) for: {incomplete.join(", ")}
        </text>
      ) : null}
      <text fg={theme.textDim} wrapMode="word">
        File overlap is a heads-up, not a guaranteed merge conflict. Committed changes only.
      </text>
    </Card>
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
    <Card
      title={`${ai ? "AI summary" : "Summary"} · ${slice}`}
      width={86}
      hints={[
        { key: "j/k", label: "scroll" },
        { key: "s", label: "summary" },
        { key: "S", label: "force AI" },
        { key: "esc", label: "close" },
      ]}
    >
      {loading ? (
        <text fg={theme.textDim} wrapMode="none">
          {ai ? "asking claude…" : "reading commit log…"}
        </text>
      ) : (
        <>
          {start > 0 ? (
            <text fg={theme.textFaint} wrapMode="none">
              ↑ {start} more above
            </text>
          ) : null}
          {shown.map((l, i) => (
            <text key={i} fg={theme.text} wrapMode="none">
              {l === "" ? " " : l}
            </text>
          ))}
          {start + maxRows < lines.length ? (
            <text fg={theme.textFaint} wrapMode="none">
              ↓ {lines.length - start - maxRows} more below
            </text>
          ) : null}
        </>
      )}
    </Card>
  );
}

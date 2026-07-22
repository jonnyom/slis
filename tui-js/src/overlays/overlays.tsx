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
import { CodePreview } from "../components/codepreview";
import { Spinner } from "../components/spinner";
import { TextField } from "../components/textfield";
import { BOLD } from "../components/ui";
import { stripSgr } from "../util/ansi";
import { visibleTextLines } from "./textinput";
import { stackHelpItems } from "./stack";

export function SwapOverlay({
  slice,
  active,
  replacing,
}: {
  slice: string;
  active: boolean;
  replacing?: string;
}): ReactNode {
  const detail = replacing
    ? `Restores ${replacing}'s primaries, then activates ${slice}. Dirty primary work is safely stashed.`
    : active
    ? "Restores each primary to its previous branch."
    : "Puts each primary on slis/live/" +
      slice +
      " at the slice tip; dirty work is stashed and restored on swap-out.";
  return (
    <Card
      title={replacing ? `Swap · ${replacing} → ${slice}` : `Swap · ${slice}`}
      width={68}
      hints={[
        { key: "y", label: "confirm" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <text wrapMode="none">
        <span fg={theme.text}>{replacing ? "swap OUT " : active ? "swap OUT " : "swap IN "}</span>
        <span fg={theme.textBright} attributes={BOLD}>
          {replacing ?? slice}
        </span>
        {replacing ? (
          <>
            <span fg={theme.text}>{" · then swap IN "}</span>
            <span fg={theme.textBright} attributes={BOLD}>{slice}</span>
          </>
        ) : null}
        <span fg={theme.text}>?</span>
      </text>
      <text fg={theme.textDim} wrapMode="word">
        {detail}
      </text>
    </Card>
  );
}

export function EditorPickerOverlay({
  editors,
  sel,
  slice,
  repo,
  path,
  line,
}: {
  editors: EditorSpec[];
  sel: number;
  slice: string;
  repo?: string;
  path?: string;
  line?: number;
}): ReactNode {
  const target = path
    ? `${slice} · ${repo} · ${path}${line ? `:${line}` : ""}`
    : repo
      ? `${slice} · ${repo}`
      : slice;
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
  mode,
  agents,
  sel,
  slice,
  preferredAgent,
}: {
  mode: "launch" | "configure" | "review";
  agents: AgentSpec[];
  sel: number;
  slice?: string;
  preferredAgent?: string;
}): ReactNode {
  const configuring = mode === "configure";
  const reviewing = mode === "review";
  return (
    <Card
      title={
        configuring
          ? "Agent settings"
          : reviewing
            ? "Review with which agent?"
            : "Launch which agent?"
      }
      subtitle={configuring ? "Choose the default launch agent" : slice}
      width={58}
      hints={configuring
        ? [
            { key: "↑/↓", label: "select" },
            { key: "enter", label: "set default" },
            { key: "esc", label: "cancel" },
          ]
        : [
            { key: "1-9", label: "quick pick" },
            { key: "↑/↓", label: "select" },
            { key: "enter", label: reviewing ? "review" : "launch" },
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
            {a.name === preferredAgent ? <span fg={theme.good}>  default</span> : null}
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
        <text key={i} wrapMode="word" fg={theme.text}>
          {l === "" ? " " : stripSgr(l)}
        </text>
      ))}
    </Card>
  );
}

export function StackActionsOverlay({
  slices,
  conflictWith,
  gatherable,
}: {
  slices: string[];
  conflictWith: string[];
  gatherable: boolean;
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
        ...(gatherable ? [{ key: "g", label: "gather" }] : []),
        { key: "x", label: "scatter" },
        { key: "?", label: "help" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {conflictWith.length > 0 ? (
        <text fg={theme.attn} wrapMode="word">
          {glyph.overlap} {target} shares changed files with: {conflictWith.join(", ")} (may be
          stale; committed changes only)
        </text>
      ) : null}
      {gatherable ? (
        <text fg={theme.textDim} wrapMode="word">
          restack runs across all targets; submit / merge / sync / gather / scatter act on the first
          target. gather folds the first target's whole Graphite stack into it (scatter undoes it).
        </text>
      ) : (
        <text fg={theme.textDim} wrapMode="word">
          {target} is standalone, so it cannot be gathered. Stack labels apply to the slices beneath
          them.
        </text>
      )}
    </Card>
  );
}

export function StackActionsHelpOverlay({
  target,
  gatherable,
}: {
  target: string;
  gatherable: boolean;
}): ReactNode {
  return (
    <Card
      title="Stack actions · help"
      subtitle={`target: ${target}`}
      width={82}
      hints={[
        { key: "?", label: "back" },
        { key: "esc", label: "back" },
      ]}
    >
      {stackHelpItems(gatherable).map((item) => (
        <box key={item.key} flexDirection="row" width="100%">
          <text wrapMode="none" fg={theme.focus} attributes={BOLD}>
            {`${item.key} ${item.label}`.padEnd(15)}
          </text>
          <text flexGrow={1} wrapMode="word" fg={theme.text}>
            {item.detail}
          </text>
        </box>
      ))}
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
      <TextField
        id="create-slice-name"
        label="New slice name"
        lines={[text]}
        description="Creates a worktree per repo (off each trunk)."
      />
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
      title={slices.length === 1 ? "Group / rename slice" : "Group slices"}
      subtitle={`selected: ${slices.join(", ")}`}
      width={60}
      hints={[
        { key: "enter", label: "group" },
        { key: "esc", label: "cancel" },
      ]}
    >
      <TextField id="group-name" label="Group name" lines={[text]} />
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

// The comment composer: captured context (repo · branch · file:line range) +
// the exact selected lines and a capped, auto-scrolling wrapped input.
export function CommentComposerOverlay({
  ctx,
  text,
}: {
  ctx: CommentContext;
  text: string;
}): ReactNode {
  const excerpt = ctx.hunk ? ctx.hunk.split("\n").slice(0, 6) : [];
  const location = `${ctx.file}:${ctx.line}${ctx.endLine && ctx.endLine > ctx.line ? `-${ctx.endLine}` : ""}`;
  const side = ctx.side === "old" ? " · old/deleted" : " · new/added";
  const inputLines = visibleTextLines(text, 68, 5);
  return (
    <Card
      title={ctx.endLine && ctx.endLine > ctx.line ? "Comment on selected lines" : "Comment on selected line"}
      subtitle={`${ctx.repo} · ${ctx.branch || "?"} ${glyph.arrow} ${location}${side}`}
      width={76}
      hints={[
        { key: "enter", label: "add comment" },
        { key: "esc", label: "cancel" },
      ]}
    >
      {excerpt.length > 0 ? (
        <box marginBottom={1}>
          <CodePreview id="comment-code-preview" lines={excerpt} path={ctx.file} />
        </box>
      ) : null}
      <TextField id="comment-input" label="Comment for the agent" lines={inputLines} />
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
          Starts the configured agent if needed, delivers one structured prompt, then clears the batch.
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
        { key: "a", label: "agent review" },
        { key: "?", label: "more" },
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
                  {c.author ? <span fg={theme.focus}>{c.author} </span> : null}
                  <span fg={focused ? theme.textBright : theme.text}>
                    {c.file}:{c.line}
                    {c.end_line && c.end_line > c.line ? `-${c.end_line}` : ""}
                    {c.side === "old" ? " (old/deleted)" : ""}
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

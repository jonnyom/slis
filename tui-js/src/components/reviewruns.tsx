import type { ReactNode } from "react";
import type { ReviewRun } from "../rpc/types";
import { wrapText } from "../pr/comments";
import { color, glyph, theme } from "../theme";
import { Panel } from "./panel";
import { BOLD, DIM } from "./ui";

function reviewRunColor(status: ReviewRun["status"]): string {
  if (status === "running" || status === "queued") return theme.focus;
  if (status === "failed") return theme.bad;
  if (status === "findings") return theme.attn;
  return theme.good;
}

export function ReviewsSection({
  runs,
  focused,
  selected,
}: {
  runs: ReviewRun[];
  focused: boolean;
  selected: number;
}): ReactNode {
  const latest = runs[0];
  const start = Math.min(Math.max(0, selected - 1), Math.max(0, runs.length - 2));
  return (
    <Panel
      title="Reviews"
      variant="seamless"
      focused={focused}
      trailing={latest ? latest.status : "none"}
    >
      {runs.length === 0 ? (
        <text fg={color.dim} attributes={DIM}>
          press n to start
        </text>
      ) : (
        runs.slice(start, start + 2).map((run, index) => (
          <text key={run.id} wrapMode="none">
            <span fg={color.cursorBar}>
              {focused && selected === start + index ? glyph.focusBar : " "}
            </span>
            <span fg={reviewRunColor(run.status)}>{run.status}</span>
            <span fg={color.dim}>{"  " + run.agent}</span>
            {run.finding_count > 0 ? (
              <span fg={theme.attn}>{`  ${run.finding_count} findings`}</span>
            ) : null}
          </text>
        ))
      )}
    </Panel>
  );
}

export function ReviewsRight({
  runs,
  selected,
  width,
}: {
  runs: ReviewRun[];
  selected: number;
  width: number;
}): ReactNode {
  const run = runs[selected];
  if (!run) {
    return (
      <>
        <text fg={theme.textBright} attributes={BOLD}>
          No review conversations yet
        </text>
        <text fg={theme.textDim}>Press n to start a persistent agent review.</text>
      </>
    );
  }
  return (
    <>
      <text wrapMode="none">
        <span fg={reviewRunColor(run.status)} attributes={BOLD}>
          {run.status}
        </span>
        <span fg={theme.textBright}>{"  " + run.agent}</span>
        <span fg={theme.textDim}>
          {`  ${run.finding_count} finding${run.finding_count === 1 ? "" : "s"}`}
        </span>
      </text>
      <text fg={theme.textFaint} wrapMode="none">
        {run.id}
      </text>
      <text> </text>
      {(run.messages ?? []).map((message) => (
        <box key={message.id} flexDirection="column">
          <text
            fg={
              message.role === "reviewer"
                ? theme.focus
                : message.role === "system"
                  ? theme.bad
                  : theme.textDim
            }
            attributes={BOLD}
          >
            {message.role === "reviewer" ? run.agent : message.role}
          </text>
          {message.body
            .split("\n")
            .flatMap((line) => wrapText(line, width))
            .map((line, index) => (
              <text key={`${message.id}-${index}`} fg={theme.text} wrapMode="word">
                {line || " "}
              </text>
            ))}
          <text> </text>
        </box>
      ))}
      {run.error ? <text fg={theme.bad}>{run.error}</text> : null}
    </>
  );
}

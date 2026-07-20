import { useKeyboard } from "@opentui/react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import type { SliceView } from "../state/derive";
import type { StatusEntry } from "../rpc/types";
import {
  isShellCmd,
  killTmuxSession,
  listTmuxSessions,
  tmuxSessionRelatedToMembers,
  type TmuxSessionInfo,
} from "../term/tmux";
import { color, glyph, theme } from "../theme";
import { BOLD, DIM } from "./ui";
import { normalizeKeyName } from "../util/keys";

function relatedSlice(session: TmuxSessionInfo, views: SliceView[]): string | null {
  for (const view of views) {
    const members = view.slice.members.map((member) => ({
      repo: member.repo,
      branch: member.branch,
      worktreePath: member.worktree_path,
    }));
    if (tmuxSessionRelatedToMembers(session, members)) return view.slice.name;
  }
  return null;
}

export interface SessionRow {
  session?: TmuxSessionInfo;
  slice: string | null;
  recovery?: StatusEntry;
}

export function buildSessionRows(
  sessions: TmuxSessionInfo[],
  views: SliceView[],
  statusEntries: StatusEntry[],
): SessionRow[] {
  const rows: SessionRow[] = sessions.map((session) => ({
    session,
    slice: relatedSlice(session, views),
  }));
  for (const entry of statusEntries) {
    if (!entry.session_id || (entry.status !== "waiting-input" && entry.status !== "done")) continue;
    const view = views.find((candidate) => candidate.slice.name === entry.slice);
    if (!view) continue;
    const members = view.slice.members.map((member) => ({
      repo: member.repo,
      branch: member.branch,
      worktreePath: member.worktree_path,
    }));
    const related = rows.find(
      (row) =>
        row.session?.kind === "agent" &&
        tmuxSessionRelatedToMembers(row.session, members),
    );
    if (related?.session?.panes.some((pane) => !isShellCmd(pane.command))) continue;
    if (related) related.recovery = entry;
    else rows.push({ slice: entry.slice, recovery: entry });
  }
  return rows;
}

export function SessionOverlay({
  enabled,
  views,
  statusEntries,
  onClose,
  onAttach,
  onResume,
}: {
  enabled: boolean;
  views: SliceView[];
  statusEntries: StatusEntry[];
  onClose: () => void;
  onAttach: (slice: string | null, session: string) => void;
  onResume: (entry: StatusEntry) => void;
}): ReactNode {
  const [sessions, setSessions] = useState<TmuxSessionInfo[]>([]);
  const [selected, setSelected] = useState(0);
  const [pendingKill, setPendingKill] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const rows = useMemo(
    () => buildSessionRows(sessions, views, statusEntries),
    [sessions, views, statusEntries],
  );
  const selectedRow = rows[Math.min(selected, Math.max(0, rows.length - 1))];

  useEffect(() => {
    if (!enabled) return;
    let live = true;
    const refresh = () =>
      listTmuxSessions().then((next) => {
        if (!live) return;
        setSessions(next);
        setSelected((current) => Math.max(0, Math.min(current, next.length - 1)));
      });
    void refresh();
    const interval = setInterval(refresh, 3_000);
    return () => {
      live = false;
      clearInterval(interval);
    };
  }, [enabled]);

  useKeyboard((key) => {
    if (!enabled) return;
    const name = normalizeKeyName(key);
    if (pendingKill) {
      if (name === "y" || name === "enter" || name === "return") {
        const target = pendingKill;
        killTmuxSession(target).then((closed) => {
          setPendingKill(null);
          setStatus(closed ? `Closed ${target}` : `Could not close ${target}`);
          if (closed) setSessions((current) => current.filter((session) => session.name !== target));
        });
      } else if (name === "n" || name === "escape") {
        setPendingKill(null);
      }
      return;
    }
    if (name === "escape" || name === "q" || name === "s") return onClose();
    if (name === "j" || name === "down") {
      setSelected((current) => Math.min(rows.length - 1, current + 1));
      return;
    }
    if (name === "k" || name === "up") {
      setSelected((current) => Math.max(0, current - 1));
      return;
    }
    if (name === "enter" || name === "return") {
      if (selectedRow?.recovery) onResume(selectedRow.recovery);
      else if (selectedRow?.session) onAttach(selectedRow.slice, selectedRow.session.name);
      return;
    }
    if (name === "x" && selectedRow?.session) {
      setStatus(null);
      setPendingKill(selectedRow.session.name);
    }
  });

  return (
    <box
      position="absolute"
      top={0}
      left={0}
      width="100%"
      height="100%"
      alignItems="center"
      justifyContent="center"
    >
      <box
        border
        borderStyle="rounded"
        borderColor={color.boxBorder}
        title="Sessions — running & recoverable"
        titleColor={color.title}
        flexDirection="column"
        padding={1}
        width="90%"
        height="80%"
        backgroundColor={color.overlayBg}
      >
        <text fg={color.dim} attributes={DIM} wrapMode="none">
          j/k select · enter attach/resume · x close · s/esc close
        </text>
        <scrollbox flexGrow={1} scrollbarOptions={{ visible: true }}>
          {rows.length === 0 ? (
            <text fg={color.dim} attributes={DIM}>
              (no running Slis tmux sessions)
            </text>
          ) : (
            rows.map(({ session, slice, recovery }, index) => {
              const running = session?.panes.some((pane) => !isShellCmd(pane.command)) ?? false;
              const label = session?.name ?? `claude/${recovery?.session_id?.slice(0, 8)}`;
              return (
                <box key={session?.name ?? recovery?.session_id} flexDirection="column">
                  <text wrapMode="none">
                    <span fg={index === selected ? color.cursorBar : color.dim}>
                      {index === selected ? glyph.focusBar : " "}
                    </span>
                    <span fg={running ? theme.good : color.dim}>{running ? glyph.live : "·"}</span>
                    <span fg={index === selected ? color.white : color.fg} attributes={index === selected ? BOLD : 0}>
                      {` ${label}`}
                    </span>
                    {slice ? <span fg={theme.focus}>{`  ‹${slice}›`}</span> : null}
                    {recovery ? <span fg={theme.attn}>{"  resume"}</span> : null}
                  </text>
                  {(session?.panes ?? []).map((pane, paneIndex) => (
                    <text key={`${pane.path}-${paneIndex}`} fg={color.dim} attributes={DIM} wrapMode="none">
                      {`    ${pane.command}  ${pane.path}`}
                    </text>
                  ))}
                  {!session && recovery?.cwd ? (
                    <text fg={color.dim} attributes={DIM} wrapMode="none">
                      {`    ${recovery.cwd}`}
                    </text>
                  ) : null}
                </box>
              );
            })
          )}
        </scrollbox>
        {pendingKill ? (
          <text fg={theme.bad} attributes={BOLD} wrapMode="none">
            {`Close ${pendingKill}? y confirm · n cancel`}
          </text>
        ) : status ? (
          <text fg={status.startsWith("Closed") ? theme.good : theme.bad}>{status}</text>
        ) : null}
      </box>
    </box>
  );
}

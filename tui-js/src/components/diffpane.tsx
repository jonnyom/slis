// Self-contained diff pane. Takes raw diff data for one repo (stat + patch, as
// the sidecar returns them) and renders either the file stat or the unified
// patch, grouped per file via the pure parser. Wave 2's side-by-side /
// syntax-highlighted differ can replace the render here without touching the
// cockpit — it already receives parsed FileDiff[] from the same module.

import { useMemo, type ReactNode } from "react";
import type { DiffScope, DiffStat } from "../rpc/types";
import { parseUnifiedDiff, type FileDiff } from "../diff/parse";
import { color, diffColor } from "../theme";
import { BOLD, DIM } from "./ui";

const SCOPE_LABEL: Record<DiffScope, string> = {
  working: "working tree (dirty)",
  parent: "vs parent",
  trunk: "vs trunk",
};

function StatBody({ stat }: { stat: DiffStat }): ReactNode {
  return (
    <>
      {stat.files.map((f) => {
        const binary = f.added < 0 || f.deleted < 0;
        return (
          <text key={f.path} wrapMode="none">
            {binary ? (
              <span fg={color.dim}>{"  bin".padStart(10)}</span>
            ) : (
              <>
                <span fg={diffColor.add}>{`+${f.added}`.padStart(5)}</span>
                <span fg={diffColor.del}>{`-${f.deleted}`.padStart(5)}</span>
              </>
            )}
            <span fg={color.fg}> {f.path}</span>
          </text>
        );
      })}
    </>
  );
}

function FileBlock({ file }: { file: FileDiff }): ReactNode {
  return (
    <box flexDirection="column">
      <text wrapMode="none">
        <span fg={color.repoHeader} attributes={BOLD}>
          {file.oldPath && file.oldPath !== file.path
            ? `${file.oldPath} → ${file.path}`
            : file.path}
        </span>
        {file.binary ? (
          <span fg={color.dim}> (binary)</span>
        ) : (
          <>
            <span fg={diffColor.add}> +{file.added}</span>
            <span fg={diffColor.del}> -{file.deleted}</span>
          </>
        )}
      </text>
      {file.hunks.map((h, hi) => (
        <box key={hi} flexDirection="column">
          <text fg={diffColor.hunk} wrapMode="none">
            {h.header}
          </text>
          {h.lines.map((l, li) => {
            const gutter = String(l.newNumber ?? l.oldNumber ?? "").padStart(4);
            const marker = l.type === "add" ? "+" : l.type === "del" ? "-" : " ";
            const c =
              l.type === "add"
                ? diffColor.add
                : l.type === "del"
                  ? diffColor.del
                  : color.fg;
            return (
              <text key={li} fg={c} wrapMode="none">
                <span fg={color.dim}>{gutter} </span>
                {marker}
                {l.content === "" ? " " : l.content}
              </text>
            );
          })}
        </box>
      ))}
    </box>
  );
}

export function DiffPane({
  repo,
  stat,
  patch,
  err,
  scope,
  showPatch,
}: {
  repo: string;
  stat?: DiffStat | null;
  patch?: string | null;
  err?: string;
  scope: DiffScope;
  showPatch: boolean;
}): ReactNode {
  const files = useMemo(
    () => (patch ? parseUnifiedDiff(patch) : []),
    [patch],
  );

  if (err) {
    return (
      <text fg={color.missing} wrapMode="none">
        {repo}: {err}
      </text>
    );
  }
  if (!stat && !patch) {
    return (
      <text fg={color.dim} attributes={DIM}>
        no changes in {repo} ({SCOPE_LABEL[scope]})
      </text>
    );
  }

  const files0 = stat?.files ?? [];
  const totalAdded =
    stat?.added ?? files0.reduce((a, f) => a + Math.max(f.added, 0), 0);
  const totalDeleted =
    stat?.deleted ?? files0.reduce((a, f) => a + Math.max(f.deleted, 0), 0);

  return (
    <>
      <text wrapMode="none">
        <span fg={color.dim}>{files0.length || files.length} files · </span>
        <span fg={diffColor.add}>+{totalAdded}</span>
        <span fg={diffColor.del}> -{totalDeleted}</span>
        <span fg={color.dim}> · {SCOPE_LABEL[scope]}</span>
      </text>
      <text> </text>
      {showPatch ? (
        patch ? (
          files.map((f, i) => <FileBlock key={i} file={f} />)
        ) : (
          <text fg={color.dim} attributes={DIM}>
            (no patch for this scope — press t for stat)
          </text>
        )
      ) : stat ? (
        <StatBody stat={stat} />
      ) : (
        <text fg={color.dim} attributes={DIM}>
          (no stat — press t for patch)
        </text>
      )}
    </>
  );
}

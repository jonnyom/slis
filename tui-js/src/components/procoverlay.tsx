// All-slices process overlay (`P`) — parity with the Bubble Tea TUI's
// cross-slice process table. Polls procs for every slice, groups them under a
// per-slice header, and supports the same sort / kill / kill-tree keys as the
// cockpit pane. Kill runs in-process (proc/kill); the sidecar stays read-only.

import { useKeyboard } from "@opentui/react";
import { useMemo, useState, type ReactNode } from "react";
import type { ProcEntry, RpcClient } from "../rpc/types";
import type { FlatProcRow } from "../proc/tree";
import { useProcMonitor } from "../proc/monitor";
import { nextSort, SORT_LABEL, sortProcs, type ProcSort } from "../proc/sort";
import { color, glyph } from "../theme";
import { normalizeKeyName } from "../util/keys";
import { BOLD, DIM } from "./ui";
import {
  applyKill,
  KillConfirm,
  KillStatusLine,
  ProcTableHeader,
  ProcTreeRow,
  SLICE_CPU_WARN,
  type KillStatus,
  type KillTarget,
} from "./procview";

function flatRow(p: ProcEntry): FlatProcRow {
  return {
    proc: p,
    depth: 0,
    hasChildren: false,
    collapsed: false,
    subtreeCpu: p.cpu,
    subtreeMem: p.mem,
    prefix: "",
  };
}

interface Selectable {
  proc: ProcEntry;
  sliceProcs: ProcEntry[];
}

export function AllSlicesProcOverlay({
  client,
  enabled,
  onClose,
}: {
  client: RpcClient;
  enabled: boolean;
  onClose: () => void;
}): ReactNode {
  const monitor = useProcMonitor(client, undefined, true);
  const [sort, setSort] = useState<ProcSort>("cpu");
  const [sel, setSel] = useState(0);
  const [pendingKill, setPendingKill] = useState<KillTarget | null>(null);
  const [killStatus, setKillStatus] = useState<KillStatus | null>(null);

  const groups = monitor.result?.slices ?? [];
  const selectable: Selectable[] = useMemo(() => {
    const out: Selectable[] = [];
    for (const g of groups) {
      for (const p of sortProcs(g.procs, sort)) out.push({ proc: p, sliceProcs: g.procs });
    }
    return out;
  }, [groups, sort]);

  const clampedSel = Math.max(0, Math.min(sel, Math.max(0, selectable.length - 1)));

  const requestKill = (subtree: boolean) => {
    const target = selectable[clampedSel];
    if (!target) return;
    setKillStatus(null);
    setPendingKill({ pid: target.proc.pid, subtree, cmd: target.proc.cmd });
  };

  const confirmKill = () => {
    if (!pendingKill) return;
    const target = selectable.find((s) => s.proc.pid === pendingKill.pid);
    setKillStatus(applyKill(target?.sliceProcs ?? [], pendingKill));
    setPendingKill(null);
  };

  useKeyboard((key) => {
    if (!enabled) return;
    const name = normalizeKeyName(key);
    if (pendingKill) {
      if (name === "y" || name === "return" || name === "enter") return confirmKill();
      if (name === "n" || name === "escape") return setPendingKill(null);
      return;
    }
    if (name === "P" || name === "escape" || name === "q") return onClose();
    if (name === "s") return setSort((s) => nextSort(s));
    if (name === "x") return requestKill(false);
    if (name === "X") return requestKill(true);
    if (name === "j" || name === "down")
      return setSel((i) => Math.min(selectable.length - 1, i + 1));
    if (name === "k" || name === "up") return setSel((i) => Math.max(0, i - 1));
    if (name === "g") return setSel(0);
    if (name === "G") return setSel(Math.max(0, selectable.length - 1));
  });

  let runningIndex = -1;
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
        title="Processes — all slices"
        titleColor={color.title}
        flexDirection="column"
        padding={1}
        width="90%"
        height="80%"
        backgroundColor={color.overlayBg}
      >
        <text wrapMode="none" fg={color.dim} attributes={DIM}>
          {`sort: ${SORT_LABEL[sort]} · s cycle · j/k move · x kill · X kill tree · P/esc close`}
        </text>
        <ProcTableHeader spark />
        <scrollbox flexGrow={1} scrollbarOptions={{ visible: true }}>
          {selectable.length === 0 ? (
            <text fg={color.dim} attributes={DIM}>
              (no tmux sessions / no processes)
            </text>
          ) : (
            groups.map((g) => {
              const over = g.totalCPU > SLICE_CPU_WARN;
              return (
                <box key={g.slice} flexDirection="column">
                  <text wrapMode="none">
                    <span fg={color.repoHeader} attributes={BOLD}>
                      {glyph.filterMarker} {g.slice}
                    </span>
                    <span fg={over ? color.restack : color.dim}>
                      {`  Σ ${g.totalCPU.toFixed(0)}%`}
                    </span>
                    {over ? <span fg={color.restack} attributes={BOLD}> ⚠</span> : null}
                  </text>
                  {sortProcs(g.procs, sort).map((p) => {
                    runningIndex++;
                    return (
                      <ProcTreeRow
                        key={p.pid}
                        row={flatRow(p)}
                        selected={runningIndex === clampedSel}
                        history={monitor.history}
                        spark
                      />
                    );
                  })}
                </box>
              );
            })
          )}
        </scrollbox>
        {pendingKill ? (
          <KillConfirm target={pendingKill} />
        ) : killStatus ? (
          <KillStatusLine status={killStatus} />
        ) : null}
      </box>
    </box>
  );
}

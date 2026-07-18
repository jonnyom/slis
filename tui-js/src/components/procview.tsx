// Presentational pieces for the process viewer, shared by the cockpit's
// Processes pane and the all-slices `P` overlay. Kill logic runs in the JS
// front-end (proc/kill) — the Go sidecar stays read-only. Pure rendering here;
// the owning view holds the state and keyboard.

import type { ReactNode } from "react";
import type { ProcEntry } from "../rpc/types";
import type { FlatProcRow } from "../proc/tree";
import { killProc, killSubtree } from "../proc/kill";
import { sparkline } from "../proc/sparkline";
import { ProcHistory } from "../proc/history";
import { color, glyph } from "../theme";
import { BOLD, DIM } from "./ui";

export const ROW_CPU_WARN = 50; // per-proc CPU% that turns a row orange
export const SLICE_CPU_WARN = 150; // slice-total CPU% that raises the ⚠ badge
const SPARK_W = 12;

export interface KillTarget {
  pid: number;
  subtree: boolean;
  cmd: string;
}

export interface KillStatus {
  ok: boolean;
  text: string;
}

function uniqueKinds(errors: { kind: string }[]): string {
  return [...new Set(errors.map((e) => e.kind))].join("/");
}

/** Run the kill in-process and describe the outcome for the status line. */
export function applyKill(procs: ProcEntry[], target: KillTarget): KillStatus {
  const outcome = target.subtree
    ? killSubtree(procs, target.pid)
    : killProc(target.pid);
  const label = target.subtree ? `subtree of ${target.pid}` : `pid ${target.pid}`;
  if (outcome.ok && outcome.errors.length === 0) {
    const n = target.subtree ? `${outcome.killed} proc(s)` : `pid ${target.pid}`;
    return { ok: true, text: `SIGTERM → ${n}` };
  }
  if (outcome.killed > 0) {
    return {
      ok: true,
      text: `SIGTERM ${label}: ${outcome.killed} ok, ${outcome.errors.length} failed (${uniqueKinds(outcome.errors)})`,
    };
  }
  return { ok: false, text: `kill ${label} failed: ${uniqueKinds(outcome.errors)}` };
}

function fmtCpu(v: number): string {
  return v.toFixed(1);
}
function fmtMem(v: number): string {
  return v.toFixed(1);
}

function truncate(s: string, width: number): string {
  const flat = s.replace(/\s+/g, " ").trim();
  const r = [...flat];
  if (r.length <= width) return flat;
  return r.slice(0, Math.max(1, width - 1)).join("") + "…";
}

export function ProcTableHeader({ spark }: { spark: boolean }): ReactNode {
  return (
    <text wrapMode="none">
      <span fg={color.dim} attributes={DIM}>
        {"  " + "PID".padEnd(7) + "CPU%".padStart(6) + "  "}
      </span>
      {spark ? (
        <span fg={color.dim} attributes={DIM}>
          {"CPU~".padEnd(SPARK_W)}
        </span>
      ) : null}
      <span fg={color.dim} attributes={DIM}>
        {"MEM(MB)".padStart(8) + "  "}
      </span>
      {spark ? (
        <span fg={color.dim} attributes={DIM}>
          {"MEM~".padEnd(SPARK_W)}
        </span>
      ) : null}
      <span fg={color.dim} attributes={DIM}>
        CMD
      </span>
    </text>
  );
}

function SparkCell({
  values,
  fg,
}: {
  values: number[];
  fg: string;
}): ReactNode {
  const s = sparkline(values, { width: SPARK_W });
  return (
    <span fg={values.length > 1 ? fg : color.dim}>{s.padEnd(SPARK_W)}</span>
  );
}

export function ProcTreeRow({
  row,
  selected,
  history,
  spark,
}: {
  row: FlatProcRow;
  selected: boolean;
  history?: ProcHistory;
  spark: boolean;
}): ReactNode {
  const p = row.proc;
  const cpuOver = p.cpu > ROW_CPU_WARN;
  const mark = row.hasChildren ? (row.collapsed ? "▸" : "▾") : " ";
  const cmd = truncate(p.cmd, 60);
  const rollup =
    row.hasChildren && (row.collapsed || row.depth === 0)
      ? ` Σ${row.subtreeCpu.toFixed(0)}%`
      : "";
  return (
    <text wrapMode="none" attributes={selected ? BOLD : 0}>
      <span fg={color.cursorBar}>{selected ? glyph.focusBar + " " : "  "}</span>
      <span fg={selected ? color.white : color.fg}>{String(p.pid).padEnd(7)}</span>
      <span fg={cpuOver ? color.restack : color.fg}>{fmtCpu(p.cpu).padStart(6)}</span>
      <span fg={color.dim}>{"  "}</span>
      {spark ? (
        <SparkCell values={history?.cpuSeries(p.pid) ?? []} fg={color.synced} />
      ) : null}
      <span fg={color.fg}>{fmtMem(p.mem).padStart(8)}</span>
      <span fg={color.dim}>{"  "}</span>
      {spark ? (
        <SparkCell values={history?.memSeries(p.pid) ?? []} fg={color.repoHeader} />
      ) : null}
      <span fg={color.dim}>{row.prefix}</span>
      <span fg={cpuOver ? color.restack : color.dim}>{mark} </span>
      <span fg={selected ? color.white : color.fg}>{cmd}</span>
      {rollup ? <span fg={color.dim}>{rollup}</span> : null}
    </text>
  );
}

export function ProcTotalsRow({
  cpu,
  mem,
  count,
}: {
  cpu: number;
  mem: number;
  count: number;
}): ReactNode {
  const over = cpu > SLICE_CPU_WARN;
  return (
    <text wrapMode="none" attributes={BOLD}>
      <span fg={color.dim}>{"  Σ ".padEnd(9)}</span>
      <span fg={over ? color.restack : color.fg}>{fmtCpu(cpu).padStart(6)}</span>
      <span fg={over ? color.restack : color.dim}>{over ? " ⚠" : "  "}</span>
      <span fg={color.fg}>{fmtMem(mem).padStart(SPARK_W + 6)}</span>
      <span fg={color.dim}>{`  ${count} proc(s)`}</span>
    </text>
  );
}

export function KillConfirm({ target }: { target: KillTarget }): ReactNode {
  const action = target.subtree ? "kill subtree of" : "kill";
  return (
    <text wrapMode="none">
      <span fg={color.missing} attributes={BOLD}>
        {`SIGTERM — ${action} pid ${target.pid}`}
      </span>
      <span fg={color.dim}>{`  (${truncate(target.cmd, 30)})  `}</span>
      <span fg={color.synced} attributes={BOLD}>
        [y]
      </span>
      <span fg={color.fg}> yes </span>
      <span fg={color.missing} attributes={BOLD}>
        [n]
      </span>
      <span fg={color.fg}> no</span>
    </text>
  );
}

export function KillStatusLine({ status }: { status: KillStatus }): ReactNode {
  return (
    <text wrapMode="none">
      <span fg={status.ok ? color.synced : color.missing} attributes={BOLD}>
        {status.ok ? "✓ " : "✗ "}
      </span>
      <span fg={status.ok ? color.fg : color.missing}>{status.text}</span>
    </text>
  );
}

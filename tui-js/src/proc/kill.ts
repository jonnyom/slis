// Killing runs in the JS front-end, NOT the read-only Go sidecar: the sidecar
// must never mutate. We send SIGTERM via process.kill (Bun) and classify the
// errno so the UI can report ESRCH (already gone) / EPERM (not ours) instead of
// crashing. Subtree order comes from proc/tree.subtreePids (deepest-first).

import type { ProcEntry } from "../rpc/types";
import { subtreePids } from "./tree";

export type KillErrorKind = "ESRCH" | "EPERM" | "OTHER";

export interface KillOutcome {
  ok: boolean;
  /** How many signals were delivered successfully. */
  killed: number;
  /** Non-fatal failures, e.g. a child already exited (ESRCH). */
  errors: { pid: number; kind: KillErrorKind; message: string }[];
}

function classify(err: unknown): KillErrorKind {
  const code = (err as { code?: string } | null)?.code;
  if (code === "ESRCH") return "ESRCH";
  if (code === "EPERM") return "EPERM";
  return "OTHER";
}

function signalOne(pid: number, signal: NodeJS.Signals): { kind: KillErrorKind } | null {
  try {
    process.kill(pid, signal);
    return null;
  } catch (err) {
    return { kind: classify(err) };
  }
}

export function killProc(pid: number, signal: NodeJS.Signals = "SIGTERM"): KillOutcome {
  const fail = signalOne(pid, signal);
  if (fail) {
    return { ok: false, killed: 0, errors: [{ pid, kind: fail.kind, message: fail.kind }] };
  }
  return { ok: true, killed: 1, errors: [] };
}

export function killSubtree(
  procs: ProcEntry[],
  rootPid: number,
  signal: NodeJS.Signals = "SIGTERM",
): KillOutcome {
  const order = subtreePids(procs, rootPid);
  const pids = order.length > 0 ? order : [rootPid];
  let killed = 0;
  const errors: KillOutcome["errors"] = [];
  for (const pid of pids) {
    const fail = signalOne(pid, signal);
    if (fail) errors.push({ pid, kind: fail.kind, message: fail.kind });
    else killed++;
  }
  return { ok: killed > 0, killed, errors };
}

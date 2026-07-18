// React hook driving the process sampler: polls the RPC `procs` method on a
// tick while the pane is visible, feeds each snapshot into a persistent
// ProcHistory (for sparklines), and prunes dead pids. Timer lives here; the
// history/tree/sort logic stays in pure modules.

import { useEffect, useRef, useState } from "react";
import type { ProcsResult, RpcClient } from "../rpc/types";
import { ProcHistory } from "./history";

export const SAMPLE_INTERVAL_MS = 2500;
export const HISTORY_CAPACITY = 32;

export interface ProcMonitor {
  result: ProcsResult | null;
  history: ProcHistory;
  /** Bumped on every sample so consumers re-render as history grows. */
  version: number;
}

export function useProcMonitor(
  client: RpcClient,
  sliceFilter: string | undefined,
  active: boolean,
  intervalMs = SAMPLE_INTERVAL_MS,
): ProcMonitor {
  const historyRef = useRef<ProcHistory | null>(null);
  if (!historyRef.current) historyRef.current = new ProcHistory(HISTORY_CAPACITY);
  const [result, setResult] = useState<ProcsResult | null>(null);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!active) return;
    let live = true;
    const load = () =>
      client.procs(sliceFilter).then((r) => {
        if (!live) return;
        historyRef.current!.sample(r.slices.flatMap((s) => s.procs));
        setResult(r);
        setVersion((v) => v + 1);
      }, () => {});
    load();
    const id = setInterval(load, intervalMs);
    return () => {
      live = false;
      clearInterval(id);
    };
  }, [client, sliceFilter, active, intervalMs]);

  return { result, history: historyRef.current, version };
}

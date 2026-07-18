// Per-pid CPU/mem history. Each `sample` pushes the latest reading into a ring
// buffer per pid and drops pids that disappeared (dead processes), so the
// history never leaks memory across a long-lived session. Pure logic — the
// React layer owns the polling timer and simply calls `sample` on each tick.

import type { ProcEntry } from "../rpc/types";
import { RingBuffer } from "./ring";

interface Series {
  cpu: RingBuffer;
  mem: RingBuffer;
}

export class ProcHistory {
  private readonly series = new Map<number, Series>();

  constructor(private readonly capacity = 32) {}

  sample(procs: Pick<ProcEntry, "pid" | "cpu" | "mem">[]): void {
    const live = new Set<number>();
    for (const p of procs) {
      live.add(p.pid);
      let s = this.series.get(p.pid);
      if (!s) {
        s = { cpu: new RingBuffer(this.capacity), mem: new RingBuffer(this.capacity) };
        this.series.set(p.pid, s);
      }
      s.cpu.push(p.cpu);
      s.mem.push(p.mem);
    }
    for (const pid of [...this.series.keys()]) {
      if (!live.has(pid)) this.series.delete(pid);
    }
  }

  cpuSeries(pid: number): number[] {
    return this.series.get(pid)?.cpu.values() ?? [];
  }

  memSeries(pid: number): number[] {
    return this.series.get(pid)?.mem.values() ?? [];
  }

  get trackedPids(): number[] {
    return [...this.series.keys()];
  }
}

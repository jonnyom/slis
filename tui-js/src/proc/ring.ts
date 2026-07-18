// Fixed-capacity ring buffer for per-pid CPU/mem history. Oldest samples fall
// off the front once capacity is reached. Pure, dependency-free, and total.

export class RingBuffer {
  private readonly buf: number[];
  private start = 0;
  private len = 0;

  constructor(readonly capacity: number) {
    if (capacity < 1) throw new Error("RingBuffer capacity must be >= 1");
    this.buf = new Array<number>(capacity);
  }

  push(value: number): void {
    if (this.len < this.capacity) {
      this.buf[(this.start + this.len) % this.capacity] = value;
      this.len++;
      return;
    }
    // Full: overwrite the oldest and advance the window.
    this.buf[this.start] = value;
    this.start = (this.start + 1) % this.capacity;
  }

  /** Samples oldest → newest. */
  values(): number[] {
    const out = new Array<number>(this.len);
    for (let i = 0; i < this.len; i++) {
      out[i] = this.buf[(this.start + i) % this.capacity]!;
    }
    return out;
  }

  get size(): number {
    return this.len;
  }

  last(): number | undefined {
    if (this.len === 0) return undefined;
    return this.buf[(this.start + this.len - 1) % this.capacity];
  }
}

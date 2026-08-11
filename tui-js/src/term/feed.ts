export class TerminalFeedBuffer {
  private pendingChunks: Uint8Array[] = [];
  private pendingByteCount = 0;
  private quietTimer: ReturnType<typeof setTimeout> | null = null;
  private maximumTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(
    private readonly deliver: (bytes: Uint8Array) => void,
    private readonly quietDelayMilliseconds = 34,
    private readonly maximumDelayMilliseconds = 100,
  ) {}

  write(bytes: Uint8Array): void {
    this.pendingChunks.push(bytes.slice());
    this.pendingByteCount += bytes.length;
    if (this.quietTimer !== null) clearTimeout(this.quietTimer);
    this.quietTimer = setTimeout(() => this.flush(), this.quietDelayMilliseconds);
    if (this.maximumTimer === null) {
      this.maximumTimer = setTimeout(
        () => this.flush(),
        this.maximumDelayMilliseconds,
      );
    }
  }

  cancel(): void {
    if (this.quietTimer !== null) clearTimeout(this.quietTimer);
    if (this.maximumTimer !== null) clearTimeout(this.maximumTimer);
    this.quietTimer = null;
    this.maximumTimer = null;
    this.pendingChunks = [];
    this.pendingByteCount = 0;
  }

  private flush(): void {
    if (this.quietTimer !== null) clearTimeout(this.quietTimer);
    if (this.maximumTimer !== null) clearTimeout(this.maximumTimer);
    this.quietTimer = null;
    this.maximumTimer = null;
    if (this.pendingChunks.length === 0) return;

    const combinedBytes = new Uint8Array(this.pendingByteCount);
    let writeOffset = 0;
    for (const chunk of this.pendingChunks) {
      combinedBytes.set(chunk, writeOffset);
      writeOffset += chunk.length;
    }
    this.pendingChunks = [];
    this.pendingByteCount = 0;
    this.deliver(combinedBytes);
  }
}

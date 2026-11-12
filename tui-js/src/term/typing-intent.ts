import { parseKeypress } from "@opentui/core";

export const DOCK_TYPING_BURST_MILLISECONDS = 180;

const MINIMUM_DISTINCT_KEYS = 3;
const NAVIGATION_KEYS = new Set(["h", "j", "k", "l"]);

type BufferedInput = {
  key: string;
  sequence: string;
};

type DockTypingInputGuardOptions = {
  canFocusTerminal: () => boolean;
  focusTerminal: () => void;
  isTerminalFocused: () => boolean;
  replayToSlis: (sequence: string) => void;
  useKittyKeyboard: () => boolean;
  writeToTerminal: (sequence: string) => void;
};

function printableKey(sequence: string, useKittyKeyboard: boolean): string | null {
  const key = parseKeypress(sequence, { useKittyKeyboard });
  if (
    !key ||
    key.ctrl ||
    key.meta ||
    key.option ||
    key.super ||
    key.hyper ||
    key.eventType === "release"
  ) return null;
  const characters = Array.from(key.sequence);
  if (characters.length !== 1) return null;
  const codePoint = characters[0]!.codePointAt(0)!;
  return codePoint >= 0x20 && codePoint !== 0x7f ? key.sequence : null;
}

export class DockTypingInputGuard {
  private autoFocused = false;
  private bufferedInput: BufferedInput[] = [];
  private replayingInput = false;
  private terminalWasFocused = false;
  private timer: ReturnType<typeof setTimeout> | null = null;

  constructor(private readonly options: DockTypingInputGuardOptions) {}

  get terminalFocused(): boolean {
    const focused = this.options.isTerminalFocused();
    if (this.terminalWasFocused && !focused) this.autoFocused = false;
    this.terminalWasFocused = focused;
    return focused || this.autoFocused;
  }

  handle(sequence: string): boolean {
    if (this.replayingInput) return false;
    if (!this.options.canFocusTerminal()) {
      this.takeBufferedInput();
      return false;
    }
    const key = printableKey(sequence, this.options.useKittyKeyboard());
    if (!key) {
      this.replayBufferedInput();
      return false;
    }

    this.bufferedInput.push({ key, sequence });
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = setTimeout(
      () => this.expireBufferedInput(),
      DOCK_TYPING_BURST_MILLISECONDS,
    );

    const distinctKeys = new Set(this.bufferedInput.map((entry) => entry.key));
    const onlyNavigationKeys = this.bufferedInput.every((entry) =>
      NAVIGATION_KEYS.has(entry.key.toLowerCase()),
    );
    if (distinctKeys.size >= MINIMUM_DISTINCT_KEYS && !onlyNavigationKeys) {
      const bufferedInput = this.takeBufferedInput();
      this.autoFocused = true;
      this.options.focusTerminal();
      for (const entry of bufferedInput) this.options.writeToTerminal(entry.sequence);
    }
    return true;
  }

  unfocus(): void {
    this.autoFocused = false;
    this.terminalWasFocused = false;
  }

  dispose(): void {
    this.takeBufferedInput();
  }

  private replayBufferedInput(): void {
    const bufferedInput = this.takeBufferedInput();
    this.replayingInput = true;
    try {
      for (const entry of bufferedInput) this.options.replayToSlis(entry.sequence);
    } finally {
      this.replayingInput = false;
    }
  }

  private expireBufferedInput(): void {
    if (!this.options.canFocusTerminal()) {
      this.takeBufferedInput();
      return;
    }
    this.replayBufferedInput();
  }

  private takeBufferedInput(): BufferedInput[] {
    const bufferedInput = this.bufferedInput;
    this.bufferedInput = [];
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    return bufferedInput;
  }
}

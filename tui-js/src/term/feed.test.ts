import { describe, expect, test } from "bun:test";
import { TERMINAL_FRAME_DELAY_MILLISECONDS, TerminalFeedBuffer } from "./feed";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

describe("TerminalFeedBuffer", () => {
  test("waits for a fragmented terminal redraw to go quiet", async () => {
    const updates: string[] = [];
    const feedBuffer = new TerminalFeedBuffer(
      (bytes) => updates.push(decoder.decode(bytes)),
      30,
    );

    feedBuffer.write(encoder.encode("first "));
    await Bun.sleep(20);
    feedBuffer.write(encoder.encode("second "));
    await Bun.sleep(20);
    feedBuffer.write(encoder.encode("third"));

    expect(updates).toEqual([]);
    await Bun.sleep(35);
    expect(updates).toEqual(["first second third"]);
  });

  test("delivers continuous output within a bounded delay", async () => {
    const updates: string[] = [];
    const feedBuffer = new TerminalFeedBuffer(
      (bytes) => updates.push(decoder.decode(bytes)),
      50,
      80,
    );

    feedBuffer.write(encoder.encode("first "));
    await Bun.sleep(30);
    feedBuffer.write(encoder.encode("second "));
    await Bun.sleep(30);
    feedBuffer.write(encoder.encode("third"));
    await Bun.sleep(25);

    expect(updates).toEqual(["first second third"]);
  });

  test("delivers continuous animation output at UI frame cadence", async () => {
    const updates: string[] = [];
    const feedBuffer = new TerminalFeedBuffer(
      (bytes) => updates.push(decoder.decode(bytes)),
      TERMINAL_FRAME_DELAY_MILLISECONDS,
      TERMINAL_FRAME_DELAY_MILLISECONDS,
    );

    for (let frame = 0; frame < 5; frame++) {
      feedBuffer.write(encoder.encode(String(frame)));
      await Bun.sleep(10);
    }

    expect(updates.length).toBeGreaterThan(0);
    feedBuffer.cancel();
  });
});

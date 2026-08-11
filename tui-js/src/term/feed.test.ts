import { describe, expect, test } from "bun:test";
import { TerminalFeedBuffer } from "./feed";

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
});

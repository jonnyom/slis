import { describe, expect, test } from "bun:test";
import { clipboardCandidates } from "./mutate";

describe("clipboardCandidates", () => {
  test("darwin uses pbcopy", () => {
    expect(clipboardCandidates("darwin")).toEqual([{ cmd: "pbcopy", args: [] }]);
  });
  test("linux offers wl-copy then xclip then xsel", () => {
    expect(clipboardCandidates("linux").map((t) => t.cmd)).toEqual([
      "wl-copy",
      "xclip",
      "xsel",
    ]);
  });
  test("xclip / xsel carry their clipboard-selection args", () => {
    const linux = clipboardCandidates("linux");
    expect(linux.find((t) => t.cmd === "xclip")?.args).toEqual(["-selection", "clipboard"]);
    expect(linux.find((t) => t.cmd === "xsel")?.args).toEqual(["--clipboard", "--input"]);
  });
  test("unknown platform has no clipboard tool", () => {
    expect(clipboardCandidates("win32")).toEqual([]);
  });
});

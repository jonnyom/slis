import { describe, expect, test } from "bun:test";
import { parseVersion, versionAtLeast } from "./require-bun-version";

describe("require-bun-version", () => {
  test("rejects the compiler version that produced the broken slis-ui", () => {
    expect(versionAtLeast("1.3.10", "1.3.14")).toBe(false);
  });

  test("accepts the pinned and newer compiler versions", () => {
    expect(versionAtLeast("1.3.14", "1.3.14")).toBe(true);
    expect(versionAtLeast("1.4.0", "1.3.14")).toBe(true);
    expect(versionAtLeast("2.0.0", "1.3.14")).toBe(true);
  });

  test("handles prerelease suffixes and malformed versions", () => {
    expect(parseVersion("1.3.14-canary.1")).toEqual([1, 3, 14]);
    expect(parseVersion("not-a-version")).toBeNull();
    expect(versionAtLeast("not-a-version", "1.3.14")).toBe(false);
  });
});

import { describe, expect, test } from "bun:test";
import { isMethodNotFound, isSliceNotFound, METHOD_NOT_FOUND, RpcError } from "./client";

describe("isMethodNotFound (older-sidecar tolerance)", () => {
  test("true for a method-not-found RpcError", () => {
    const err = new RpcError({ code: METHOD_NOT_FOUND, message: "method not found: tree" });
    expect(isMethodNotFound(err)).toBe(true);
  });

  test("false for other RpcErrors", () => {
    const err = new RpcError({
      code: -32000,
      message: "slice not found",
      data: { kind: "slice-not-found" },
    });
    expect(isMethodNotFound(err)).toBe(false);
  });

  test("false for non-RpcError values", () => {
    expect(isMethodNotFound(new Error("boom"))).toBe(false);
    expect(isMethodNotFound(undefined)).toBe(false);
    expect(isMethodNotFound({ code: METHOD_NOT_FOUND })).toBe(false);
  });
});

describe("isSliceNotFound", () => {
  test("recognizes a vanished slice response", () => {
    const err = new RpcError({
      code: -32000,
      message: "slice not found",
      data: { kind: "slice-not-found" },
    });
    expect(isSliceNotFound(err)).toBe(true);
  });

  test("rejects unrelated and non-RPC errors", () => {
    expect(isSliceNotFound(new RpcError({ code: -32000, message: "boom" }))).toBe(false);
    expect(isSliceNotFound(new Error("slice-not-found"))).toBe(false);
  });
});

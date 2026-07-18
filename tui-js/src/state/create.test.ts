import { describe, expect, test } from "bun:test";
import { createBusyLabel, createReducer, initialCreateState } from "./create";

describe("createReducer", () => {
  test("start moves idle → creating with the name", () => {
    const s = createReducer(initialCreateState, { type: "start", name: "tips-v2" });
    expect(s).toEqual({ status: "creating", name: "tips-v2" });
  });

  test("finish moves creating → idle", () => {
    const creating = createReducer(initialCreateState, { type: "start", name: "x" });
    expect(createReducer(creating, { type: "finish" })).toEqual({ status: "idle" });
  });

  test("start over an in-flight create swaps the name", () => {
    const first = createReducer(initialCreateState, { type: "start", name: "a" });
    expect(createReducer(first, { type: "start", name: "b" })).toEqual({
      status: "creating",
      name: "b",
    });
  });
});

describe("createBusyLabel", () => {
  test("idle → null", () => {
    expect(createBusyLabel(initialCreateState)).toBeNull();
  });

  test("creating → ambient label", () => {
    expect(createBusyLabel({ status: "creating", name: "invoice-pdf" })).toBe(
      "creating invoice-pdf…",
    );
  });
});

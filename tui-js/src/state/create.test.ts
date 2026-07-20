import { describe, expect, test } from "bun:test";
import {
  createBusyLabel,
  createReducer,
  initialCreateState,
  resolveCreatedSliceName,
} from "./create";

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

describe("resolveCreatedSliceName", () => {
  test("maps a prefixed requested branch to its stripped discovered slice name", () => {
    expect(
      resolveCreatedSliceName(
        {
          slices: [
            {
              name: "wfm-common-worker-shared-lib",
              base: "",
              active: false,
              stale: false,
              members: [
                {
                  repo: "web",
                  branch: "jonny/wfm-common-worker-shared-lib",
                  worktree_path: "/w",
                  tip_sha: "abc",
                },
              ],
            },
          ],
        },
        "jonny/wfm-common-worker-shared-lib",
      ),
    ).toBe("wfm-common-worker-shared-lib");
  });
});

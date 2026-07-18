import { describe, expect, test } from "bun:test";
import type { Slice } from "../rpc/types";
import type { SliceView } from "./derive";
import { isPhantom, isPhantomBranch } from "./derive";

function member(branch: string) {
  return { repo: "web", branch, worktree_path: "/tmp/x", tip_sha: "deadbeef" };
}

function view(name: string, branches: string[]): SliceView {
  return {
    slice: {
      name,
      base: "",
      active: false,
      stale: false,
      members: branches.map(member),
    } as Slice,
    status: "none",
  };
}

describe("isPhantomBranch", () => {
  test("doubled prefix is phantom", () => {
    expect(isPhantomBranch("jonny/jonny/payroll", "jonny/payroll")).toBe(true);
    expect(isPhantomBranch("a/a/x", "a/x")).toBe(true);
  });

  test("normal single-prefix branch is not phantom", () => {
    expect(isPhantomBranch("jonny/payroll", "payroll")).toBe(false);
  });

  test("no prefix (name equals branch) is not phantom", () => {
    expect(isPhantomBranch("payroll", "payroll")).toBe(false);
  });

  test("similar-but-not-doubled prefix is not phantom", () => {
    expect(isPhantomBranch("feature/feature-x", "feature-x")).toBe(false);
  });

  test("branch not ending with name is not phantom", () => {
    expect(isPhantomBranch("jonny/other", "payroll")).toBe(false);
    expect(isPhantomBranch("", "payroll")).toBe(false);
  });
});

describe("isPhantom", () => {
  test("true when any member is phantom", () => {
    expect(isPhantom(view("jonny/x", ["jonny/x", "jonny/jonny/x"]))).toBe(true);
  });
  test("false when all members are clean", () => {
    expect(isPhantom(view("x", ["jonny/x"]))).toBe(false);
  });
});

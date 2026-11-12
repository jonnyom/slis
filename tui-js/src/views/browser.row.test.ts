import { expect, test } from "bun:test";

import type { SliceView } from "../state/derive";
import { glyph, theme } from "../theme";
import { sliceRowPresentation } from "./browser";

const readyRunningSlice: SliceView = {
  slice: {
    name: "employee-not-found",
    base: "master",
    active: false,
    stale: false,
    members: [],
  },
  status: "running",
  prs: [{ repo: "Web-App", branch: "jonny/employee-not-found", number: 11157, state: "MERGED" }],
};

test("sidebar highlights a ready slice while its agent is running", () => {
  expect(sliceRowPresentation(readyRunningSlice, false)).toEqual({
    nameColor: theme.good,
    nameBold: true,
    readyGlyph: glyph.ready,
  });
});

test("focused sidebar row keeps the ready highlight", () => {
  expect(sliceRowPresentation(readyRunningSlice, true)).toEqual({
    nameColor: theme.good,
    nameBold: true,
    readyGlyph: glyph.ready,
  });
});

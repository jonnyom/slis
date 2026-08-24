import { expect, test } from "bun:test";

import { feedbackTargets } from "./feedback";
import type { SessionGroup } from "./slis";

test("feedback targets include working agents and a new-agent choice", () => {
  const groups: SessionGroup[] = [{
    id: "feature",
    active_tab_id: "root",
    tabs: [
      { id: "root", kind: "root", title: "root", cwd: "/work", agent: "Codex", label: "Codex" },
      { id: "claude", kind: "agent", title: "claude", cwd: "/work", label: "/work" },
      { id: "review-codex", kind: "review", title: "Codex", cwd: "/work", agent: "Codex", label: "Codex (1)" },
    ],
  }];

  expect(feedbackTargets(groups, "feature")).toEqual([
    { kind: "existing", tabID: "root", label: "Codex" },
    { kind: "new", label: "New feedback agent…" },
  ]);
});

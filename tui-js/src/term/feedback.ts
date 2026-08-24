import type { SessionGroup } from "./slis";

export type FeedbackTarget =
  | { kind: "existing"; tabID: string; label: string }
  | { kind: "new"; label: string };

export function feedbackTargets(groups: SessionGroup[], slice: string): FeedbackTarget[] {
  const group = groups.find((candidate) => candidate.id === slice);
  const targets: FeedbackTarget[] = (group?.tabs ?? [])
    .filter((tab) => tab.agent && tab.kind !== "review")
    .map((tab) => ({ kind: "existing", tabID: tab.id, label: tab.label ?? tab.agent! }));
  targets.push({ kind: "new", label: "New feedback agent…" });
  return targets;
}

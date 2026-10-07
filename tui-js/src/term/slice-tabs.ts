import { tabKey, type TabEntry } from "./tabs";

export function removeMissingSliceTabs(tabs: TabEntry[], previousNames: string[], nextNames: string[]): TabEntry[] {
  const available = new Set(nextNames);
  const removed = new Set(previousNames.filter((name) => !available.has(name)));
  const remaining = tabs.filter((tab) => tab.kind !== "session" || !removed.has(tab.slice));
  return remaining.length === tabs.length ? tabs : remaining;
}

export function reconcileTabSelection(tabs: TabEntry[], active: string | null, docked: string | null) {
  const activeTab = tabs.find((tab) => tabKey(tab) === active);
  const dockedTab = tabs.find((tab) => tabKey(tab) === docked);
  return {
    active: activeTab ? active : null,
    docked: dockedTab?.kind === "session" ? docked : null,
  };
}

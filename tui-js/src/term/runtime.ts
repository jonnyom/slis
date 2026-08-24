import type { SessionGroup } from "./slis";
import type { TabEntry } from "./tabs";

export function applySessionRuntimeLabels(tabs: TabEntry[], groups: SessionGroup[]): TabEntry[] {
  const labels = new Map<string, string>();
  for (const group of groups) {
    for (const tab of group.tabs) {
      if (tab.label) labels.set(`${group.id}:${tab.id}`, tab.label);
    }
  }
  let changed = false;
  const updated = tabs.map((tab): TabEntry => {
    if (tab.kind !== "session" || tab.opts.targetSession) return tab;
    const runtimeLabel = labels.get(`${tab.slice}:${tab.opts.tabID}`);
    if (!runtimeLabel || runtimeLabel === tab.opts.runtimeLabel) return tab;
    changed = true;
    return { ...tab, opts: { ...tab.opts, runtimeLabel } };
  });
  return changed ? updated : tabs;
}

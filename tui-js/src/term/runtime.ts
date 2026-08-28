import type { SessionGroup } from "./slis";
import type { TabEntry } from "./tabs";

type RuntimeLoader = (signal: AbortSignal) => Promise<SessionGroup[]>;
type RuntimeConsumer = (groups: SessionGroup[]) => void;
type RuntimeErrorConsumer = (error: unknown) => void;

export class SessionRuntimeRefresher {
  private timeout: ReturnType<typeof setTimeout> | null = null;
  private activeRefresh: AbortController | null = null;
  private lastActivityAt = 0;
  private pending = false;
  private stopped = false;

  constructor(
    private readonly load: RuntimeLoader,
    private readonly apply: RuntimeConsumer,
    private readonly reportError: RuntimeErrorConsumer,
    private readonly quietMilliseconds = 250,
  ) {}

  activity(): void {
    if (this.stopped) return;
    this.lastActivityAt = Date.now();
    this.pending = true;
    this.schedule();
  }

  stop(): void {
    this.stopped = true;
    this.pending = false;
    if (this.timeout) clearTimeout(this.timeout);
    this.timeout = null;
    this.activeRefresh?.abort();
    this.activeRefresh = null;
  }

  private schedule(): void {
    if (this.stopped || !this.pending || this.timeout || this.activeRefresh) return;
    const delay = Math.max(0, this.quietMilliseconds - (Date.now() - this.lastActivityAt));
    this.timeout = setTimeout(() => {
      this.timeout = null;
      void this.refresh();
    }, delay);
  }

  private async refresh(): Promise<void> {
    if (this.stopped || !this.pending || this.activeRefresh) return;
    const remainingQuietMilliseconds = this.quietMilliseconds - (Date.now() - this.lastActivityAt);
    if (remainingQuietMilliseconds > 0) {
      this.schedule();
      return;
    }

    this.pending = false;
    const controller = new AbortController();
    this.activeRefresh = controller;
    const result = await this.load(controller.signal).then(
      (groups) => ({ ok: true as const, groups }),
      (error: unknown) => ({ ok: false as const, error }),
    );
    if (this.activeRefresh === controller) this.activeRefresh = null;
    if (!result.ok && !this.stopped && !controller.signal.aborted) {
      this.stopped = true;
      this.pending = false;
      this.reportError(result.error);
      return;
    }
    if (result.ok && !this.stopped && !controller.signal.aborted) this.apply(result.groups);
    this.schedule();
  }
}

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

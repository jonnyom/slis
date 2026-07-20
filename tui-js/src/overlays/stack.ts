export type StackOverlayAction =
  | "context-help"
  | "restack"
  | "submit"
  | "merge"
  | "sync"
  | "gather"
  | "gather-unavailable"
  | "scatter"
  | "close"
  | null;

export function stackOverlayAction(name: string, gatherable: boolean): StackOverlayAction {
  if (name === "?") return "context-help";
  if (name === "r") return "restack";
  if (name === "p") return "submit";
  if (name === "m") return "merge";
  if (name === "s") return "sync";
  if (name === "g") return gatherable ? "gather" : "gather-unavailable";
  if (name === "x") return "scatter";
  if (name === "n" || name === "escape") return "close";
  return null;
}

export interface StackHelpItem {
  key: string;
  label: string;
  detail: string;
}

export function stackHelpItems(gatherable: boolean): StackHelpItem[] {
  return [
    {
      key: "r",
      label: "restack",
      detail: "Rebase each target's Graphite stack. Dirty worktrees are refused; conflicts stay open for gt continue.",
    },
    {
      key: "p",
      label: "submit",
      detail: "Force-push the first target's stack and open or update its pull requests through Graphite.",
    },
    {
      key: "m",
      label: "merge",
      detail: "Send the first target's stack to Graphite's server-side merge queue.",
    },
    {
      key: "s",
      label: "sync",
      detail: "Run repo-wide Graphite sync. It may update trunk and delete merged branches.",
    },
    ...(gatherable
      ? [
          {
            key: "g",
            label: "gather",
            detail: "Fold the whole Graphite stack into this slice. Only slis overrides change; branches and worktrees untouched.",
          },
        ]
      : []),
    {
      key: "x",
      label: "scatter",
      detail: "Reverse a gathered slice back into per-branch slices. Only slis overrides change.",
    },
  ];
}

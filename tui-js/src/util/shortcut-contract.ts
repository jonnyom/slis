// The user-facing shortcut contract. Key reuse is valid between contexts
// (for example `d` scrolls with Ctrl in a diff and clears a slice in the hub),
// but one key must never resolve to two actions inside the same keyboard owner.
//
// Keep this catalog in sync when adding an action binding. The companion test
// rejects same-context collisions and protects the cross-view commands that
// should feel global wherever a slice is visible.

export type ShortcutBinding = Readonly<{
  action: string;
  keys: readonly string[];
}>;

export const SHORTCUT_CONTEXTS = {
  browser: [
    { action: "help", keys: ["?"] },
    { action: "processes", keys: ["P"] },
    { action: "refresh", keys: ["r"] },
    { action: "search", keys: ["/"] },
    { action: "open", keys: ["enter", "return", "l", "right"] },
    { action: "select", keys: ["space"] },
    { action: "select-all", keys: ["A"] },
    { action: "swap", keys: ["w"] },
    { action: "create", keys: ["c"] },
    { action: "import", keys: ["i"] },
    { action: "adopt", keys: ["I"] },
    { action: "group", keys: ["m"] },
    { action: "ungroup", keys: ["u"] },
    { action: "stack-actions", keys: ["R"] },
    { action: "conflicts", keys: ["!"] },
    { action: "copy-pr-stack", keys: ["Y"] },
    { action: "clear-slice", keys: ["d"] },
    { action: "configure-agents", keys: [","] },
    { action: "ci-detail", keys: ["v"] },
    { action: "fix-ci", keys: ["F"] },
    { action: "attach-agent", keys: ["a"] },
    { action: "launch-agent", keys: ["C"] },
    { action: "pending-review", keys: ["V"] },
    { action: "open-shell", keys: ["t"] },
    { action: "open-editor", keys: ["e", "o"] },
  ],
  cockpit: [
    { action: "help", keys: ["?"] },
    { action: "processes", keys: ["P"] },
    { action: "swap", keys: ["w"] },
    { action: "attach-agent", keys: ["a"] },
    { action: "launch-agent", keys: ["C"] },
    { action: "pending-review", keys: ["V"] },
    { action: "configure-agents", keys: [","] },
    { action: "open-shell", keys: ["t"] },
    { action: "open-editor", keys: ["e"] },
    { action: "open-slice-editor", keys: ["E"] },
    { action: "open-repo-editor", keys: ["o"] },
    { action: "refresh", keys: ["r"] },
    { action: "stack-actions", keys: ["R"] },
    { action: "summary", keys: ["s"] },
    { action: "ai-summary", keys: ["S"] },
    { action: "clear-slice", keys: ["d"] },
    { action: "copy-diff", keys: ["y"] },
    { action: "copy-pr-stack", keys: ["Y"] },
    { action: "open-pr", keys: ["O"] },
  ],
  "cockpit.file": [
    { action: "help", keys: ["?"] },
    { action: "attach-agent", keys: ["a"] },
    { action: "launch-agent", keys: ["C"] },
    { action: "open-shell", keys: ["t"] },
    { action: "pending-review", keys: ["V"] },
    { action: "configure-agents", keys: [","] },
    { action: "edit-line", keys: ["e"] },
    { action: "open-repo-editor", keys: ["o"] },
    { action: "open-slice-editor", keys: ["E"] },
    { action: "comment", keys: ["c"] },
  ],
  "cockpit.tree": [
    { action: "help", keys: ["?"] },
    { action: "attach-agent", keys: ["a"] },
    { action: "launch-agent", keys: ["C"] },
    { action: "open-shell", keys: ["t"] },
    { action: "pending-review", keys: ["V"] },
    { action: "configure-agents", keys: [","] },
    { action: "edit-path", keys: ["e"] },
    { action: "open-repo-editor", keys: ["o"] },
    { action: "open-slice-editor", keys: ["E"] },
  ],
  "cockpit.prs": [
    { action: "ci-detail", keys: ["v"] },
    { action: "fix-ci", keys: ["F"] },
    { action: "copy-pr-url", keys: ["y"] },
    { action: "open-pr", keys: ["O"] },
  ],
  "cockpit.processes": [
    { action: "sort", keys: ["s"] },
    { action: "kill", keys: ["x"] },
    { action: "kill-tree", keys: ["X"] },
  ],
  diff: [
    { action: "attach-agent", keys: ["a"] },
    { action: "launch-agent", keys: ["C"] },
    { action: "pending-review", keys: ["V"] },
    { action: "configure-agents", keys: [","] },
    { action: "comment", keys: ["c"] },
    { action: "toggle-layout", keys: ["t"] },
    { action: "cycle-scope", keys: ["b"] },
    { action: "select-range", keys: ["v", "space"] },
    { action: "next-hunk", keys: ["]", "n"] },
    { action: "previous-hunk", keys: ["[", "p"] },
  ],
  "agent.launch": [
    { action: "quick-pick", keys: ["1", "2", "3", "4", "5", "6", "7", "8", "9"] },
    { action: "choose", keys: ["enter", "return"] },
    { action: "next", keys: ["j", "down"] },
    { action: "previous", keys: ["k", "up"] },
    { action: "cancel", keys: ["escape", "q"] },
  ],
  "agent.configure": [
    { action: "set-default", keys: ["enter", "return"] },
    { action: "next", keys: ["j", "down"] },
    { action: "previous", keys: ["k", "up"] },
    { action: "cancel", keys: ["escape", "q"] },
  ],
  review: [
    { action: "delete-comment", keys: ["x"] },
    { action: "send-review", keys: ["s"] },
    { action: "help", keys: ["?"] },
    { action: "close", keys: ["escape", "q"] },
  ],
} as const satisfies Record<string, readonly ShortcutBinding[]>;

export type ShortcutContext = keyof typeof SHORTCUT_CONTEXTS;

export function shortcutAction(context: ShortcutContext, key: string): string | undefined {
  return SHORTCUT_CONTEXTS[context].find((binding) => binding.keys.includes(key as never))?.action;
}

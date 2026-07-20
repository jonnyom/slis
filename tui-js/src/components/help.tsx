// Help overlay: the keybindings for the current view plus the glyph legend.

import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { Card } from "./card";
import { Eyebrow } from "./eyebrow";
import { BOLD } from "./ui";

type Binding = [keys: string, help: string];

interface BindingGroup {
  label: string;
  bindings: Binding[];
}

const BROWSER_GROUPS: BindingGroup[] = [
  {
    label: "navigate",
    bindings: [
      ["tab", "toggle States rail / Slices list"],
      ["j / k", "move down / up"],
      ["1–8", "jump to filter"],
      ["g / G", "first / last slice"],
      ["n / N", "next / prev search match (while searching) · else attention slice"],
      ["enter / l", "open slice cockpit"],
      ["/", "search slices by name"],
      ["r", "refresh workspace"],
      ["T", "cycle colour theme"],
      ["?", "toggle this help"],
      ["q", "quit"],
    ],
  },
  {
    label: "act",
    bindings: [
      ["e / o", "open whole slice in editor"],
      ["w", "swap slice in / out ([s] stash dirty)"],
      ["space / A", "select one / all visible (batch ops)"],
      ["m / u", "group selected / ungroup focused"],
      ["c", "create a new slice (worktrees per repo)"],
      ["i", "import discovered candidate worktrees"],
      ["I", "adopt an arbitrary branch as a slice"],
      ["d", "clear finished slice(s): [y] remove · [f] force"],
    ],
  },
  {
    label: "stack & prs",
    bindings: [
      ["R", "stack actions: restack / submit / merge / sync"],
      ["V", "pending-review overlay: list comments · x delete · s send to agent"],
      ["v", "open cockpit PRs panel + failing-CI log"],
      ["F", "fix-ci: point the agent at failing CI"],
      ["Y", "copy PR-stack markdown to clipboard"],
      ["!", "conflict radar (files changed by >1 slice)"],
    ],
  },
  {
    label: "session",
    bindings: [
      ["a", "open the slice's agent terminal"],
      ["C", "launch agent + open terminal tab (picks when multiple are available)"],
      [",", "configure the default launch agent"],
      ["t", "open a separate persistent shell terminal"],
      ["P", "processes across all slices"],
    ],
  },
];

const COCKPIT_GROUPS: BindingGroup[] = [
  {
    label: "navigate",
    bindings: [
      ["tab", "next panel"],
      ["1–4", "Repos&Stack / PRs / Session / Processes"],
      ["j / k", "move selection (Stack: any branch in the stack)"],
      ["enter / l", "open rich diff (Stack panel)"],
      ["enter", "zoom right pane full-width (other panels)"],
      ["b", "cycle diff scope working → parent → trunk (member branch)"],
      ["ctrl+d / ctrl+u", "scroll right pane"],
      ["g / G", "top / bottom of right pane"],
      ["T", "cycle colour theme"],
      ["esc / h", "back to browser"],
      ["q", "quit"],
    ],
  },
  {
    label: "stack review",
    bindings: [
      ["j / k", "select any branch → right pane shows its diff vs its stack parent"],
      ["f", "browse the selected branch's files at that revision"],
      ["j / k · l / enter", "in the file tree: move · expand dir / preview file"],
      ["e", "in the file tree/file view: edit selection at the current line"],
      ["o / E", "open selected repo worktree / whole slice workspace"],
      ["h", "in the file tree: collapse dir (or its parent)"],
      ["esc", "step back: file → tree → diff → browser"],
    ],
  },
  {
    label: "review",
    bindings: [
      ["c", "focus rich-diff lines, then comment on the selected line / range"],
      ["V", "pending-review overlay: list comments · x delete · s send to agent"],
    ],
  },
  {
    label: "act",
    bindings: [
      ["e / o / E", "editor: contextual selection / repo worktree / whole slice"],
      ["w", "swap slice in / out (live)"],
      ["d", "clear this finished slice"],
    ],
  },
  {
    label: "stack & prs",
    bindings: [
      ["R", "stack actions: restack / submit / merge / sync"],
      ["y (Stack) / Y", "copy focused diff / PR-stack markdown"],
      ["y (PRs)", "copy focused PR URL"],
      ["v", "failing CI log in right pane (PRs panel)"],
      ["ctrl+r", "re-run failed CI runs (PRs panel)"],
      ["F", "fix-ci: point the agent at failing CI (PRs panel)"],
      ["O", "open focused PR in browser"],
    ],
  },
  {
    label: "session",
    bindings: [
      ["h / l", "collapse / expand process subtree"],
      ["s", "cycle process sort cpu → mem → pid"],
      ["x / X", "kill process / kill subtree (SIGTERM)"],
      ["P", "processes across all slices"],
      ["a", "open the slice's agent terminal"],
      ["C", "launch an available coding agent"],
      [",", "configure the default launch agent"],
      ["t", "open a separate persistent shell terminal"],
      ["S", "force AI summary (s: summary outside Processes panel)"],
    ],
  },
];

const DIFF_BINDINGS: Binding[] = [
  ["j / k", "move in the focused file list or diff lines"],
  ["[ / ]  ·  p / n", "prev / next hunk"],
  ["Tab / Enter", "move focus between the file list and diff lines"],
  ["v / Space", "toggle multi-line selection, then extend it with j / k"],
  ["c", "comment on the selected diff line / range (feeds the agent)"],
  ["V", "pending-review overlay (list / delete / send)"],
  ["C", "launch an available coding agent"],
  [",", "configure the default launch agent"],
  ["a", "attach to the slice agent without leaving the diff"],
  ["h / l", "in side-by-side mode, select old/deleted or new/added side"],
  ["t", "toggle unified / side-by-side"],
  ["b", "cycle diff scope"],
  ["ctrl+d / ctrl+u", "scroll diff"],
  ["g / G", "top / bottom"],
  ["esc", "back to the file list, then cockpit"],
];

function BindingRows({ bindings }: { bindings: Binding[] }): ReactNode {
  // A fixed key column + a flex-growing description that wraps rather than
  // clips (M2): every hidden key is reachable only through this screen, so no
  // description may be truncated at any terminal width.
  return (
    <>
      {bindings.map(([keys, help], i) => (
        <box key={i} flexDirection="row" width="100%">
          <text wrapMode="none" fg={theme.focus} attributes={BOLD}>
            {keys.padEnd(18)}
          </text>
          <text flexGrow={1} wrapMode="word" fg={theme.text}>
            {help}
          </text>
        </box>
      ))}
    </>
  );
}

function Group({ label, bindings }: BindingGroup): ReactNode {
  return (
    <>
      <Eyebrow label={label} bar={false} />
      <BindingRows bindings={bindings} />
    </>
  );
}

function Legend(): ReactNode {
  const items: Array<[string, string, string]> = [
    [glyph.waiting, "waiting for you", theme.attn],
    [glyph.done, "done", theme.merged],
    [glyph.ready, "ready to clear", theme.good],
    [glyph.inReview, "in review", theme.focus],
    [glyph.live, "live / running", theme.good],
    [glyph.restack, "needs restack", theme.attn],
    [glyph.stale, "primary behind (stale)", theme.attn],
    [glyph.overlap, "file overlap (>1 slice)", theme.attn],
    [glyph.idle, "idle", theme.textDim],
  ];
  return (
    <text wrapMode="word">
      {items.map(([g, label, c], i) => (
        <span key={i}>
          <span fg={c} attributes={BOLD}>
            {g}
          </span>
          <span fg={theme.textDim}>
            {" "}
            {label}
            {i < items.length - 1 ? "   " : ""}
          </span>
        </span>
      ))}
    </text>
  );
}

export function Help({ view }: { view: "browser" | "cockpit" }): ReactNode {
  const groups = view === "browser" ? BROWSER_GROUPS : COCKPIT_GROUPS;
  const title = view === "browser" ? "slis — Browser shortcuts" : "slis — Cockpit shortcuts";
  return (
    <Card title={title} width={84} hints={[{ key: "esc", label: "close" }]}>
      {groups.map((group) => (
        <Group key={group.label} label={group.label} bindings={group.bindings} />
      ))}
      {view === "cockpit" ? (
        <>
          <Eyebrow label="rich diff" bar={false} />
          <BindingRows bindings={DIFF_BINDINGS} />
        </>
      ) : null}
      <Legend />
      <text fg={theme.textDim} wrapMode="word">
        in a terminal tab: ctrl+q returns here · tmux detach is C-b d · ? / esc to close
      </text>
    </Card>
  );
}

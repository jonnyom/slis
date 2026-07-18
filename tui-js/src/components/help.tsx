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
      ["n / N", "next / prev slice needing you"],
      ["enter / l", "open slice cockpit"],
      ["/", "search slices by name"],
      ["r", "refresh workspace"],
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
      ["Y", "copy PR-stack markdown to clipboard"],
      ["!", "conflict radar (files changed by >1 slice)"],
    ],
  },
  {
    label: "session",
    bindings: [
      ["a", "open session terminal tab"],
      ["C", "launch agent + open terminal tab"],
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
      ["j / k", "move selection in panel"],
      ["enter / l", "open rich diff (Stack panel)"],
      ["enter", "zoom right pane full-width (other panels)"],
      ["b", "cycle diff scope working → parent → trunk"],
      ["t", "toggle stat / patch"],
      ["ctrl+d / ctrl+u", "scroll right pane"],
      ["g / G", "top / bottom of right pane"],
      ["esc / h", "back to browser"],
      ["q", "quit"],
    ],
  },
  {
    label: "act",
    bindings: [
      ["e / o", "editor: whole slice / selected repo"],
      ["w", "swap slice in / out (live)"],
      ["d", "clear this finished slice"],
    ],
  },
  {
    label: "stack & prs",
    bindings: [
      ["R", "stack actions: restack / submit / merge / sync"],
      ["y / Y", "yank diff / PR-stack markdown to clipboard"],
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
      ["a / C", "open terminal tab (C also launches agent)"],
      ["S", "force AI summary (s: summary outside Processes panel)"],
    ],
  },
];

const DIFF_BINDINGS: Binding[] = [
  ["j / k", "next / prev file"],
  ["[ / ]  ·  p / n", "prev / next hunk"],
  ["t", "toggle unified / side-by-side"],
  ["b", "cycle diff scope"],
  ["ctrl+d / ctrl+u", "scroll diff"],
  ["g / G", "top / bottom"],
  ["esc / h", "back to cockpit"],
];

function BindingRows({ bindings }: { bindings: Binding[] }): ReactNode {
  return (
    <>
      {bindings.map(([keys, help], i) => (
        <text key={i} wrapMode="none">
          <span fg={theme.focus} attributes={BOLD}>
            {keys.padEnd(18)}
          </span>
          <span fg={theme.text}>{help}</span>
        </text>
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
    [glyph.idle, "idle", theme.textDim],
  ];
  return (
    <text wrapMode="none">
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
    <Card title={title} width={60} hints={[{ key: "esc", label: "close" }]}>
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
      <text fg={theme.textDim} wrapMode="none">
        in a terminal tab: ctrl+q returns here · tmux detach is C-b d · ? / esc to close
      </text>
    </Card>
  );
}

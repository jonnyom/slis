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

// Every row here must match a real handler (views/browser.tsx, views/cockpit.tsx,
// components/diffview.tsx, app.tsx). Keep descriptions short enough to fit on
// one line in the 84-column card: this screen is read at a glance.
const BROWSER_GROUPS: BindingGroup[] = [
  {
    label: "navigate",
    bindings: [
      ["j / k · g / G", "move · first / last"],
      ["enter / l", "open the slice"],
      ["tab · 1–8", "switch to the filters · jump to a filter"],
      ["/", "search by name (n / N step through matches)"],
      ["n / N", "next / previous slice that needs you"],
      ["r · T", "refresh · cycle colour theme"],
      ["? · q", "this help · quit"],
    ],
  },
  {
    label: "slices",
    bindings: [
      ["space / A", "select one / all visible (actions use the selection)"],
      ["c · i · I", "create a slice · import worktrees · adopt a branch"],
      ["w", "swap into the primary checkouts, or back out"],
      ["e / o", "open the slice in your editor"],
      ["d", "clear finished slice(s)"],
      ["m / u", "merge selected into one slice / split it back"],
    ],
  },
  {
    label: "units (related slices, listed last)",
    bindings: [
      ["U", "put selected slices in a unit (new or existing name)"],
      ["U, empty name", "take selected slices out of their unit"],
      ["enter", "on a unit header: fold / unfold"],
      ["h / l", "fold / unfold the unit you're in"],
    ],
  },
  {
    label: "agents & terminals",
    bindings: [
      ["a", "show / hide the slice's agent"],
      ["C · L · ,", "launch the default agent · pick one · set the default"],
      ["t", "open a shell"],
      ["s · P", "all sessions · all processes"],
    ],
  },
  {
    label: "stack & prs",
    bindings: [
      ["R", "restack / submit / merge / sync"],
      ["v · F", "failing CI log · ask the agent to fix CI"],
      ["V", "review comments (x delete · s send to agent)"],
      ["Y", "copy all PRs + diffs as Markdown"],
      ["!", "files changed by more than one slice"],
    ],
  },
];

const COCKPIT_GROUPS: BindingGroup[] = [
  {
    label: "navigate",
    bindings: [
      ["tab / 1–5", "Stack · PRs · Reviews · Session · Processes"],
      ["j / k", "move the selection"],
      ["enter", "Stack: open diff · Session: attach · others: zoom"],
      ["ctrl+d/u · g/G", "scroll the right pane · top / bottom"],
      ["r · T", "refresh · cycle colour theme"],
      ["esc · q", "back to the browser · quit"],
    ],
  },
  {
    label: "slice",
    bindings: [
      ["a", "show / hide the slice's agent"],
      ["C · L · ,", "launch the default agent · pick one · set the default"],
      ["t", "open a shell"],
      ["e / E · o", "open the slice in your editor · open the repo"],
      ["w · d", "swap in / out · clear finished slice"],
      ["R · Y", "restack / submit / merge / sync · copy all PRs"],
      ["s · S", "summary · AI summary"],
      ["V · P", "review comments · all processes"],
    ],
  },
  {
    label: "stack panel",
    bindings: [
      ["b", "diff against: working → parent → trunk"],
      ["y", "copy the slice diff"],
      ["f", "browse files (l open · h close · esc back)"],
      ["e · c", "in a file: edit / comment at the cursor line"],
    ],
  },
  {
    label: "other panels",
    bindings: [
      ["v · ctrl+r · F", "PRs: CI log · re-run failed CI · ask agent to fix"],
      ["y · O", "PRs: copy URL · open in browser"],
      ["n · m", "Reviews: start a review · message the run"],
      ["x", "Session: close it · Processes: kill (X: tree)"],
      ["h / l · s", "Processes: fold / unfold · cycle sort"],
    ],
  },
];

const DIFF_BINDINGS: Binding[] = [
  ["j / k · tab", "move · switch file list / diff lines"],
  ["n / p", "next / previous hunk (also ] / [)"],
  ["v / space", "select a range, extend with j / k"],
  ["c", "comment on the line or range (feeds the agent)"],
  ["t · h / l", "unified / side-by-side · pick a side"],
  ["b", "cycle diff scope"],
  ["2", "open this file's PR"],
  ["esc", "back to the file list, then the cockpit"],
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
    [glyph.waiting, "needs you", theme.attn],
    [glyph.running, "agent working", theme.good],
    [glyph.subagents, "sub-agents working", theme.good],
    [glyph.done, "agent done", theme.focus],
    [glyph.live, "swapped in", theme.good],
    [glyph.prOpen, "PR in review", theme.focus],
    [glyph.approved, "approved", theme.good],
    [glyph.changes, "changes requested", theme.bad],
    [glyph.ready, "merged", theme.merged],
    [glyph.restack, "needs restack", theme.attn],
    [glyph.stale, "stale", theme.attn],
    [glyph.overlap, "file overlap", theme.attn],
    [glyph.idle, "idle", theme.textDim],
  ];
  // Each item is its own unwrapped text in a wrapping row, so a label like
  // "needs restack" never splits across lines.
  return (
    <box flexDirection="row" flexWrap="wrap" width="100%">
      {items.map(([g, label, c], i) => (
        <text key={i} wrapMode="none" marginRight={3}>
          <span fg={c} attributes={BOLD}>
            {g}
          </span>
          <span fg={theme.textDim}> {label}</span>
        </text>
      ))}
    </box>
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
        in a terminal: ctrl+shift+← / → switch tabs · ctrl+q back · ctrl+g unfocus
      </text>
    </Card>
  );
}

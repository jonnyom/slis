// Help overlay: the keybindings for the current view plus the glyph legend.

import type { ReactNode } from "react";
import { color, glyph } from "../theme";
import { Overlay } from "./overlay";
import { BOLD, DIM } from "./ui";

type Binding = [keys: string, help: string];

const BROWSER_BINDINGS: Binding[] = [
  ["tab", "toggle States rail / Slices list"],
  ["j / k", "move down / up"],
  ["1–8", "jump to filter"],
  ["g / G", "first / last slice"],
  ["enter / l", "open slice cockpit"],
  ["w", "swap slice in / out (live)"],
  ["space / A", "select one / all visible (batch ops)"],
  ["m / u", "group selected / ungroup focused"],
  ["R", "stack actions: restack / submit / merge / sync"],
  ["c", "create a new slice (worktrees per repo)"],
  ["i / I", "import / adopt discovered candidate worktrees"],
  ["d", "clear finished slice(s): [y] remove · [f] force"],
  ["Y", "copy PR-stack markdown to clipboard"],
  ["!", "conflict radar (files changed by >1 slice)"],
  ["/", "search slices by name"],
  ["r", "refresh workspace"],
  ["?", "toggle this help"],
  ["q", "quit"],
];

const COCKPIT_BINDINGS: Binding[] = [
  ["tab", "next panel"],
  ["1–4", "Repos&Stack / PRs / Session / Processes"],
  ["j / k", "move selection in panel"],
  ["b", "cycle diff scope working → parent → trunk"],
  ["t", "toggle stat / patch"],
  ["ctrl+d / ctrl+u", "scroll right pane"],
  ["g / G", "top / bottom of right pane"],
  ["w", "swap slice in / out (live)"],
  ["s / S", "summary (commit) / force AI summary"],
  ["R", "stack actions: restack / submit / merge / sync"],
  ["d", "clear this finished slice"],
  ["y / Y", "yank diff / PR-stack markdown to clipboard"],
  ["O", "open focused PR in browser"],
  ["esc / h", "back to browser"],
  ["q", "quit"],
];

function Legend(): ReactNode {
  const items: Array<[string, string, string]> = [
    [glyph.waiting, "waiting for you", color.wait],
    [glyph.done, "done", color.done],
    [glyph.ready, "ready to clear", color.ready],
    [glyph.inReview, "in review", color.synced],
    [glyph.live, "live / running", color.live],
    [glyph.idle, "idle", color.dim],
  ];
  return (
    <text wrapMode="none">
      {items.map(([g, label, c], i) => (
        <span key={i}>
          <span fg={c} attributes={BOLD}>
            {g}
          </span>
          <span fg={color.dim}>
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
  const bindings = view === "browser" ? BROWSER_BINDINGS : COCKPIT_BINDINGS;
  const title = view === "browser" ? "slis — Browser shortcuts" : "slis — Cockpit shortcuts";
  return (
    <Overlay title={title} width={54}>
      {bindings.map(([keys, help], i) => (
        <text key={i} wrapMode="none">
          <span fg={color.candidate} attributes={BOLD}>
            {keys.padEnd(18)}
          </span>
          <span fg={color.fg}>{help}</span>
        </text>
      ))}
      <text> </text>
      <Legend />
      <text> </text>
      <text fg={color.dim} attributes={DIM} wrapMode="none">
        tmux detach is C-b d (not Ctrl-D) · ? / esc to close
      </text>
    </Overlay>
  );
}

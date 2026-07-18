// Contextual key hints — the 4–6 actions relevant to the current focus (spec
// §1.4, §3.4). Renders `key` in focus + `label` in textDim, always ending in
// `? more`. Never wraps: truncates with `…` before it would overflow the width.

import { useTerminalDimensions } from "@opentui/react";
import type { ReactNode } from "react";
import { theme } from "../theme";

export interface Hint {
  key: string;
  label: string;
}

const MORE: Hint = { key: "?", label: "more" };
const GAP = "   ";
const GAP_LEN = GAP.length;

function segLen(h: Hint): number {
  return h.key.length + 1 + h.label.length; // "key label"
}

// Pure fit: how many leading hints survive at `width`, and whether we truncated.
// `? more` always shows at the tail; `…` marks any omission.
export function fitHints(
  hints: Hint[],
  width: number,
): { shown: Hint[]; truncated: boolean } {
  const shown: Hint[] = [];
  let used = segLen(MORE); // "? more" is always rendered at the end
  let truncated = false;
  for (const h of hints) {
    const cost = GAP_LEN + segLen(h);
    if (used + cost <= width) {
      used += cost;
      shown.push(h);
    } else {
      truncated = true;
      break;
    }
  }
  if (truncated) {
    const ellipsisCost = GAP_LEN + 1; // "   …"
    while (shown.length > 0 && used + ellipsisCost > width) {
      const dropped = shown.pop() as Hint;
      used -= GAP_LEN + segLen(dropped);
    }
  }
  return { shown, truncated };
}

export function HintBar({
  hints,
  width,
}: {
  hints: Hint[];
  width?: number;
}): ReactNode {
  const { width: termWidth } = useTerminalDimensions();
  const avail = width ?? termWidth - 1;
  const { shown, truncated } = fitHints(hints, avail);
  return (
    <text wrapMode="none">
      {shown.map((h, i) => (
        <span key={i}>
          {i > 0 ? <span>{GAP}</span> : null}
          <span fg={theme.focus}>{h.key}</span>
          <span fg={theme.textDim}> {h.label}</span>
        </span>
      ))}
      {truncated ? (
        <span fg={theme.textFaint}>
          {GAP}…
        </span>
      ) : null}
      <span>
        {shown.length > 0 || truncated ? GAP : ""}
        <span fg={theme.focus}>{MORE.key}</span>
        <span fg={theme.textDim}> {MORE.label}</span>
      </span>
    </text>
  );
}

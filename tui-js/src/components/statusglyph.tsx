// The attention glyph for a slice — the scannable "left edge" (spec §1, §2).
// Wraps `attention()`; a level-3 row renders bold in its semantic hue so it pops.

import type { ReactNode } from "react";
import { attention } from "../theme";
import type { SliceView } from "../state/derive";
import { BOLD } from "./ui";

export function StatusGlyph({ view }: { view: SliceView }): ReactNode {
  const a = attention(view);
  return (
    <span fg={a.color} attributes={a.bold ? BOLD : 0}>
      {a.glyph}
    </span>
  );
}

// Cockpit header: `slis › slice › section` with trailing badges (spec §3.2).
// `slis` in focus, slice in textBright, the focused section name in textBright,
// separators dim. Trailing content (live/stale badge, `esc back  ? help`) sits
// right-aligned.

import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { BOLD } from "./ui";

export function Breadcrumb({
  slice,
  section,
  trailing,
}: {
  slice?: string;
  section?: string;
  trailing?: ReactNode;
}): ReactNode {
  const sep = ` ${glyph.arrow} `;
  return (
    <box flexDirection="row" justifyContent="space-between" width="100%">
      <text wrapMode="none">
        <span fg={theme.focus} attributes={BOLD}>
          slis
        </span>
        {slice ? (
          <>
            <span fg={theme.textDim}>{sep}</span>
            <span fg={theme.textBright} attributes={BOLD}>
              {slice}
            </span>
          </>
        ) : null}
        {section ? (
          <>
            <span fg={theme.textDim}>{sep}</span>
            <span fg={theme.textBright}>{section}</span>
          </>
        ) : null}
      </text>
      {trailing !== undefined && trailing !== null ? (
        <box flexDirection="row">{trailing}</box>
      ) : null}
    </box>
  );
}

// Dim uppercase section label — the debox lever (spec §2, §4). Brightens and
// grows a `▎` focus bar when focused; an optional right-aligned `trailing`
// summary lets each section telegraph its headline (`2 repos`, `⏸ waiting`).

import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { BOLD } from "./ui";

export function Eyebrow({
  label,
  focused = false,
  trailing,
  bar = true,
}: {
  label: string;
  focused?: boolean;
  trailing?: ReactNode;
  bar?: boolean;
}): ReactNode {
  const labelColor = focused ? theme.textBright : theme.textFaint;
  const prefix = bar ? (focused ? `${glyph.focusBar} ` : "  ") : "";
  return (
    <box flexDirection="row" justifyContent="space-between" width="100%">
      <text wrapMode="none">
        {prefix ? <span fg={theme.focus}>{prefix}</span> : null}
        <span fg={labelColor} attributes={focused ? BOLD : 0}>
          {label.toUpperCase()}
        </span>
      </text>
      {trailing !== undefined && trailing !== null ? (
        <box flexDirection="row">
          {typeof trailing === "string" ? (
            <text wrapMode="none" fg={focused ? theme.textDim : theme.textFaint}>
              {trailing}
            </text>
          ) : (
            trailing
          )}
        </box>
      ) : null}
    </box>
  );
}

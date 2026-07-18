// A panel with two looks (spec §4, the debox lever):
//   "bordered"  — the lazygit rounded box that brightens when focused. Default,
//                 so existing call sites are unchanged (cockpit right pane).
//   "seamless"  — no box: an `Eyebrow` (with its `▎` focus bar) + a hairline,
//                 grouped by whitespace (cockpit left sections, browser rail).

import type { ReactNode } from "react";
import { color } from "../theme";
import { Eyebrow } from "./eyebrow";

export function Panel({
  title,
  index,
  focused = false,
  variant = "bordered",
  trailing,
  children,
  flexGrow,
  height,
  width,
}: {
  title: string;
  index?: number;
  focused?: boolean;
  variant?: "seamless" | "bordered";
  trailing?: ReactNode;
  children: ReactNode;
  flexGrow?: number;
  height?: number;
  width?: number;
}): ReactNode {
  const label = index !== undefined ? `${index} ${title}` : title;

  if (variant === "seamless") {
    return (
      <box
        flexDirection="column"
        flexGrow={flexGrow}
        height={height}
        width={width}
        overflow="hidden"
      >
        <Eyebrow label={label} focused={focused} trailing={trailing} />
        {children}
      </box>
    );
  }

  return (
    <box
      border
      borderStyle="rounded"
      borderColor={focused ? color.borderFocus : color.border}
      title={label}
      titleColor={focused ? color.borderFocus : color.dim}
      flexDirection="column"
      flexGrow={flexGrow}
      height={height}
      width={width}
      paddingLeft={1}
      paddingRight={1}
      overflow="hidden"
    >
      {children}
    </box>
  );
}

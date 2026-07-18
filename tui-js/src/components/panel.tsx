// A bordered panel whose border/title brighten when focused — the lazygit look
// of the cockpit's stacked left panels.

import type { ReactNode } from "react";
import { color } from "../theme";

export function Panel({
  title,
  index,
  focused = false,
  children,
  flexGrow,
  height,
  width,
}: {
  title: string;
  index?: number;
  focused?: boolean;
  children: ReactNode;
  flexGrow?: number;
  height?: number;
  width?: number;
}): ReactNode {
  const label = index !== undefined ? `${index} ${title}` : title;
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

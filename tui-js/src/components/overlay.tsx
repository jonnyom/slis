// A centered modal card floating over the current view. Uses absolute
// positioning so it overlaps whatever is behind it.

import { useTerminalDimensions } from "@opentui/react";
import type { ReactNode } from "react";
import { color } from "../theme";
import { BOLD } from "./ui";

export function Overlay({
  title,
  children,
  width,
}: {
  title: string;
  children: ReactNode;
  width?: number;
}): ReactNode {
  // Clamp the card to the terminal so a wide overlay (e.g. the 86-col summary)
  // never overflows a narrow screen (80x24). Leave a two-cell margin.
  const { width: termWidth } = useTerminalDimensions();
  const clamped = width === undefined ? undefined : Math.min(width, termWidth - 2);
  return (
    <box
      position="absolute"
      top={0}
      left={0}
      width="100%"
      height="100%"
      alignItems="center"
      justifyContent="center"
    >
      <box
        border
        borderStyle="rounded"
        borderColor={color.boxBorder}
        title={title}
        titleColor={color.title}
        flexDirection="column"
        padding={1}
        width={clamped}
        overflow="hidden"
        backgroundColor={color.overlayBg}
      >
        {children}
      </box>
    </box>
  );
}

export function OverlayTitle({ children }: { children: ReactNode }): ReactNode {
  return (
    <text fg={color.title} attributes={BOLD} wrapMode="none">
      {children}
    </text>
  );
}

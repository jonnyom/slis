// A centered modal card floating over the current view. Uses absolute
// positioning so it overlaps whatever is behind it.

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
        width={width}
        backgroundColor="#101010"
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

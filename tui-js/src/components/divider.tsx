// Full-width hairline. A stacked-section divider can't be a per-side border
// (that can't sit mid-column, spec §6), so it's a `─`-filled text row. A
// vertical rule is a per-side `border` on the neighbouring box instead.

import type { ReactNode } from "react";
import { theme } from "../theme";

export function Divider({
  width,
  color = theme.hairline,
}: {
  width?: number;
  color?: string;
}): ReactNode {
  if (width && width > 0) {
    return (
      <text wrapMode="none" fg={color}>
        {"─".repeat(width)}
      </text>
    );
  }
  return (
    <box width="100%" height={1} overflow="hidden">
      <text wrapMode="none" fg={color}>
        {"─".repeat(500)}
      </text>
    </box>
  );
}

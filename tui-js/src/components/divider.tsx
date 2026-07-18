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
  const line = width && width > 0 ? "─".repeat(width) : "";
  return (
    <text wrapMode="none" fg={color}>
      {width && width > 0 ? line : "─".repeat(200)}
    </text>
  );
}

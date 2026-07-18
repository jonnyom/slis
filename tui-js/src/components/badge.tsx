// Small state token — glyph + text in one semantic hue (spec §4). Either pass a
// `state` keyword (resolved through `badgeFor`) or explicit glyph/color/label.

import type { ReactNode } from "react";
import { badgeFor, type BadgeState } from "../theme";
import { BOLD } from "./ui";

export function Badge(
  props:
    | { state: BadgeState; label?: string; bold?: boolean }
    | { glyph: string; color: string; label?: string; bold?: boolean },
): ReactNode {
  const spec = "state" in props ? badgeFor(props.state) : props;
  const label = props.label ?? spec.label;
  return (
    <span fg={spec.color} attributes={props.bold ? BOLD : 0}>
      {spec.glyph}
      {label ? ` ${label}` : ""}
    </span>
  );
}

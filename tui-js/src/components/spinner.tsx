// Braille spinner — motion only marks async work (spec §1.5). Extracted from
// WorkingOverlay so every "work running" affordance shares one component.

import { useEffect, useState, type ReactNode } from "react";
import { theme } from "../theme";
import { BOLD } from "./ui";

export const SPINNER_FRAMES = [
  "⠋",
  "⠙",
  "⠹",
  "⠸",
  "⠼",
  "⠴",
  "⠦",
  "⠧",
  "⠇",
  "⠏",
] as const;

export function Spinner({
  color = theme.focus,
  intervalMs = 90,
}: {
  color?: string;
  intervalMs?: number;
}): ReactNode {
  const [frame, setFrame] = useState(0);
  useEffect(() => {
    const id = setInterval(
      () => setFrame((f) => (f + 1) % SPINNER_FRAMES.length),
      intervalMs,
    );
    return () => clearInterval(id);
  }, [intervalMs]);
  return (
    <span fg={color} attributes={BOLD}>
      {SPINNER_FRAMES[frame]}
    </span>
  );
}

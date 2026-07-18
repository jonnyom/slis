// Static dim backdrop behind overlays (spec §3.3, §6). A full-screen absolute
// box at a fixed opacity — never animated (opacity is a buffer post-op; tweening
// it would repaint the whole backdrop every frame). Dim appears/disappears with
// the modal, no tween.

import type { ReactNode } from "react";
import { theme } from "../theme";

export function Scrim({
  opacity = 0.5,
  zIndex = 10,
}: {
  opacity?: number;
  zIndex?: number;
}): ReactNode {
  return (
    <box
      position="absolute"
      top={0}
      left={0}
      width="100%"
      height="100%"
      backgroundColor={theme.bg}
      opacity={opacity}
      zIndex={zIndex}
    />
  );
}

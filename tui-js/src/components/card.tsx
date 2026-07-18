// Overlay shell (spec §3.3, §4) — the one card every modal shares. Centred,
// `surface` fill, single rounded `focus` border, padding 1, a `Scrim` behind it
// so the modal pops. Header (title `focus` bold + optional subtitle), body, then
// an optional HintBar row. A `status` adds the result left-bar + ✓/✗ title
// (good on success, bad on failure) for ResultOverlay.

import { useTerminalDimensions } from "@opentui/react";
import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { HintBar, type Hint } from "./hintbar";
import { Scrim } from "./scrim";

export type CardStatus = "success" | "failure";

export function Card({
  title,
  subtitle,
  status,
  width,
  hints,
  scrim = true,
  children,
}: {
  title: string;
  subtitle?: string;
  status?: CardStatus;
  width?: number;
  hints?: Hint[];
  scrim?: boolean;
  children: ReactNode;
}): ReactNode {
  const { width: termWidth } = useTerminalDimensions();
  const clamped = width === undefined ? undefined : Math.min(width, termWidth - 2);

  const statusColor =
    status === "success" ? theme.good : status === "failure" ? theme.bad : undefined;
  const statusGlyph =
    status === "success" ? glyph.inReview : status === "failure" ? glyph.changes : "";
  const titleColor = statusColor ?? theme.focus;
  const titleText = statusColor ? `${statusGlyph} ${title}` : title;

  return (
    <box
      position="absolute"
      top={0}
      left={0}
      width="100%"
      height="100%"
      alignItems="center"
      justifyContent="center"
      zIndex={20}
    >
      {scrim ? <Scrim /> : null}
      <box
        border
        borderStyle="rounded"
        borderColor={theme.focus}
        title={titleText}
        titleColor={titleColor}
        flexDirection="column"
        padding={1}
        width={clamped}
        overflow="hidden"
        backgroundColor={theme.surface}
        zIndex={20}
      >
        {statusColor ? (
          <box border={["left"]} borderColor={statusColor} paddingLeft={1} flexDirection="column">
            {subtitle ? SubtitleRow(subtitle) : null}
            {children}
          </box>
        ) : (
          <>
            {subtitle ? SubtitleRow(subtitle) : null}
            {children}
          </>
        )}
        {hints && hints.length > 0 ? (
          <box marginTop={1}>
            <HintBar hints={hints} width={clamped ? clamped - 2 : undefined} />
          </box>
        ) : null}
      </box>
    </box>
  );
}

function SubtitleRow(subtitle: string): ReactNode {
  return (
    <text wrapMode="none" fg={theme.textDim}>
      {subtitle}
    </text>
  );
}

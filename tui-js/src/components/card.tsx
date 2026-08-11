// Overlay shell (spec §3.3, §4) — the one card every modal shares. Centred,
// `surface` fill, single rounded `focus` border, padding 1, a `Scrim` behind it
// so the modal pops. Header (title `focus` bold + optional subtitle), body, then
// an optional HintBar row. A `status` adds the result left-bar + ✓/✗ title
// (good on success, bad on failure) for ResultOverlay.

import { useTerminalDimensions } from "@opentui/react";
import type { ReactNode } from "react";
import { resultStatusStyle, theme, type ResultStatus } from "../theme";
import { HintBar, type Hint } from "./hintbar";
import { Scrim } from "./scrim";

export type CardStatus = ResultStatus;

export function Card({
  id,
  title,
  subtitle,
  status,
  width,
  hints,
  scrim = true,
  borderColor = theme.focus,
  titleColor: titleColorOverride,
  backgroundColor = theme.surface,
  paddingTop = 1,
  paddingBottom = 1,
  children,
}: {
  id?: string;
  title: string;
  subtitle?: string;
  status?: CardStatus;
  width?: number;
  hints?: Hint[];
  scrim?: boolean;
  borderColor?: string;
  titleColor?: string;
  backgroundColor?: string;
  paddingTop?: number;
  paddingBottom?: number;
  children: ReactNode;
}): ReactNode {
  const { width: termWidth } = useTerminalDimensions();
  const clamped = width === undefined ? undefined : Math.min(width, termWidth - 2);

  const statusStyle = status ? resultStatusStyle(status) : undefined;
  const statusColor = statusStyle?.color;
  const statusGlyph = statusStyle?.glyph ?? "";
  const titleColor = titleColorOverride ?? statusColor ?? theme.focus;
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
        id={id}
        border
        borderStyle="rounded"
        borderColor={borderColor}
        title={titleText}
        titleColor={titleColor}
        flexDirection="column"
        padding={1}
        paddingTop={paddingTop}
        paddingBottom={paddingBottom}
        width={clamped}
        overflow="hidden"
        backgroundColor={backgroundColor}
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
          <box id="modal-actions" marginTop={1}>
            <HintBar hints={hints} width={clamped ? clamped - 4 : undefined} />
          </box>
        ) : null}
      </box>
    </box>
  );
}

function SubtitleRow(subtitle: string): ReactNode {
  return (
    <box id="modal-subtitle" marginBottom={1}>
      <text wrapMode="none" fg={theme.textDim}>
        {subtitle}
      </text>
    </box>
  );
}

import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { BOLD } from "./ui";

export function TextField({
  id,
  label,
  lines,
  description,
}: {
  id: string;
  label: string;
  lines: string[];
  description?: string;
}): ReactNode {
  const visibleLines = lines.length > 0 ? lines : [""];
  return (
    <box flexDirection="column">
      <text id={`${id}-label`} fg={theme.textDim} attributes={BOLD} wrapMode="none">
        {label}
      </text>
      <box
        id={`${id}-control`}
        border
        borderStyle="rounded"
        borderColor={theme.focus}
        flexDirection="column"
        paddingLeft={1}
        paddingRight={1}
        backgroundColor={theme.surfaceAlt}
      >
        {visibleLines.map((line, index) => (
          <text key={index} wrapMode="none">
            <span fg={theme.textBright}>{line}</span>
            {index === visibleLines.length - 1 ? (
              <span fg={theme.focus} attributes={BOLD}>{glyph.focusBar}</span>
            ) : null}
          </text>
        ))}
      </box>
      {description ? (
        <box marginTop={1}>
          <text fg={theme.textDim} wrapMode="word">{description}</text>
        </box>
      ) : null}
    </box>
  );
}

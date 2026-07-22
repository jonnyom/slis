import type { ReactNode } from "react";
import { langForPath, tokenizeLine, type Token } from "../diff/tokenize";
import { colorForKind, diffColor, theme } from "../theme";
import { stripSgr } from "../util/ansi";

export interface PreviewLineTokens {
  prefix: "" | "+" | "-";
  tokens: Token[];
}

export function previewLineTokens(line: string, path: string): PreviewLineTokens {
  const clean = stripSgr(line);
  const prefix = clean.startsWith("+") ? "+" : clean.startsWith("-") ? "-" : "";
  const content = prefix ? clean.slice(1) : clean;
  return { prefix, tokens: tokenizeLine(content, langForPath(path)) };
}

export function CodePreview({
  id,
  lines,
  path,
}: {
  id: string;
  lines: string[];
  path: string;
}): ReactNode {
  return (
    <box id={id} flexDirection="column" backgroundColor={theme.surfaceAlt} paddingLeft={1} paddingRight={1}>
      {lines.map((line, index) => {
        const preview = previewLineTokens(line, path);
        const diffForeground = preview.prefix === "+" ? diffColor.add : preview.prefix === "-" ? diffColor.del : theme.textDim;
        const diffBackground = preview.prefix === "+" ? diffColor.addBg : preview.prefix === "-" ? diffColor.delChangeBg : undefined;
        return (
          <text key={index} wrapMode="none">
            {preview.prefix ? <span fg={diffForeground} bg={diffBackground}>{preview.prefix}</span> : null}
            {preview.tokens.length === 0 ? (
              <span bg={diffBackground}> </span>
            ) : (
              preview.tokens.map((token, tokenIndex) => (
                <span key={tokenIndex} fg={colorForKind(token.kind)} bg={diffBackground}>{token.text}</span>
              ))
            )}
          </text>
        );
      })}
    </box>
  );
}

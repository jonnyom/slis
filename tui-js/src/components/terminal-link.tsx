import type { ReactNode } from "react";
import { theme } from "../theme";
import { UNDERLINE } from "./ui";

export function TerminalLink({ url }: { url: string }): ReactNode {
  return (
    <text selectable={true} wrapMode="none">
      <a href={url} fg={theme.focus} attributes={UNDERLINE}>
        {url}
      </a>
    </text>
  );
}

import { useKeyboard } from "@opentui/react";
import type { KeyEvent } from "@opentui/core";
import type { ReactNode } from "react";
import { theme } from "../theme";
import { Card } from "./card";
import { BULK_LOAD_THRESHOLD } from "../state/bulkload";

export function BulkLoadOverlay({
  count,
  enabled,
  onLoadAll,
  onLazy,
}: {
  count: number;
  enabled: boolean;
  onLoadAll: () => void;
  onLazy: () => void;
}): ReactNode {
  useKeyboard((key: KeyEvent) => {
    if (!enabled) return;
    if (key.name === "y") onLoadAll();
    else if (key.name === "n" || key.name === "escape") onLazy();
  });

  return (
    <Card
      title="Large workspace"
      subtitle={`${count} slices — over the ${BULK_LOAD_THRESHOLD} cold-load limit`}
      width={64}
      hints={[
        { key: "y", label: "load all now" },
        { key: "n", label: "lazy — load as you go" },
      ]}
    >
      <text wrapMode="none" fg={theme.text}>
        Fetching PR + stack data for every slice up front is expensive.
      </text>
      <text wrapMode="none" fg={theme.textDim}>
        [y] fetch everything now · [n / esc] load only the focused slice, on demand.
      </text>
    </Card>
  );
}

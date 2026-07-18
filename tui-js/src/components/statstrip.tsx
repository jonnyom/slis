// Browser header attention summary (spec §3.1). The `slis` wordmark, then only
// the non-zero urgent counts in their semantic hue; calm ("N slices") when
// nothing is urgent. Version right-aligned in textFaint.

import type { ReactNode } from "react";
import { glyph, theme } from "../theme";
import { BOLD } from "./ui";

export interface StatCounts {
  needsYou?: number;
  live?: number;
  ready?: number;
  restack?: number;
  errors?: number;
}

interface Segment {
  glyph: string;
  color: string;
  count: number;
  label: string;
}

// Pure: the non-zero urgent segments, in priority order.
export function statSegments(counts: StatCounts): Segment[] {
  const all: Segment[] = [
    { glyph: glyph.waiting, color: theme.attn, count: counts.needsYou ?? 0, label: "need you" },
    { glyph: glyph.live, color: theme.good, count: counts.live ?? 0, label: "live" },
    { glyph: glyph.ready, color: theme.good, count: counts.ready ?? 0, label: "ready" },
    { glyph: glyph.restack, color: theme.attn, count: counts.restack ?? 0, label: "restack" },
    { glyph: glyph.dirty, color: theme.bad, count: counts.errors ?? 0, label: "errors" },
  ];
  return all.filter((s) => s.count > 0);
}

export function StatStrip({
  counts,
  total,
  version,
}: {
  counts: StatCounts;
  total: number;
  version?: string;
}): ReactNode {
  const segments = statSegments(counts);
  return (
    <box flexDirection="row" justifyContent="space-between" width="100%">
      <text wrapMode="none">
        <span fg={theme.focus} attributes={BOLD}>
          slis
        </span>
        {segments.length === 0 ? (
          <span fg={theme.textDim}>{`    ${total} slices`}</span>
        ) : (
          segments.map((s, i) => (
            <span key={i}>
              <span fg={theme.textFaint}>{"    "}</span>
              <span fg={s.color} attributes={BOLD}>
                {s.glyph}
              </span>
              <span fg={s.color}>{` ${s.count} `}</span>
              <span fg={theme.textDim}>{s.label}</span>
            </span>
          ))
        )}
      </text>
      {version ? (
        <text wrapMode="none" fg={theme.textFaint}>
          {version}
        </text>
      ) : null}
    </box>
  );
}

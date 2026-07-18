// Unicode sparkline for CPU/mem history. Maps each sample onto a 5-level ramp
// normalised against [min, max]. Pure and total: empty input → empty string,
// a flat series → the lowest tick, non-finite samples clamp to the floor.

const TICKS = ["▁", "▂", "▃", "▅", "▇"] as const;

export interface SparkOptions {
  /** Lower bound of the scale. Defaults to the smallest sample (relative axis). */
  min?: number;
  /** Upper bound of the scale. Defaults to the largest sample (never below min). */
  max?: number;
  /** Keep only the most recent `width` samples. */
  width?: number;
}

export function sparkline(values: number[], opts: SparkOptions = {}): string {
  let series = values.filter((v) => Number.isFinite(v));
  if (opts.width !== undefined && series.length > opts.width) {
    series = series.slice(series.length - opts.width);
  }
  if (series.length === 0) return "";

  const dataMin = series.reduce((a, b) => Math.min(a, b), series[0]!);
  const dataMax = series.reduce((a, b) => Math.max(a, b), series[0]!);
  const min = opts.min ?? dataMin;
  const max = Math.max(opts.max ?? dataMax, min);
  const span = max - min;

  return series
    .map((v) => {
      if (span <= 0) return TICKS[0];
      const clamped = Math.min(Math.max(v, min), max);
      const frac = (clamped - min) / span;
      const idx = Math.min(TICKS.length - 1, Math.round(frac * (TICKS.length - 1)));
      return TICKS[idx];
    })
    .join("");
}

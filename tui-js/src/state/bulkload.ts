export const BULK_LOAD_THRESHOLD = 25;

export type BulkPhase = "unprompted" | "all" | "lazy";

export interface BulkPlan {
  prompt: boolean;
  fanOut: boolean;
}

export function bulkLoadPlan(count: number, phase: BulkPhase): BulkPlan {
  if (count <= BULK_LOAD_THRESHOLD) return { prompt: false, fanOut: true };
  if (phase === "all") return { prompt: false, fanOut: true };
  if (phase === "lazy") return { prompt: false, fanOut: false };
  return { prompt: true, fanOut: false };
}

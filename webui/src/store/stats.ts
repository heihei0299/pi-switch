import { useCallback, useEffect, useRef } from "react";
import { api } from "../api";
import type { UsageStats } from "../types";
import type { StatsRange } from "../lib/statsWindow";
import { swrKeyStats } from "../lib/swrKeys";
import { mutate } from "swr";

export const STATS_REFRESH_TIERS = [
  { label: "Off", ms: null as number | null },
  { label: "5s", ms: 5000 },
  { label: "30s", ms: 30_000 },
  { label: "5min", ms: 300_000 },
] as const;

export function statsSWRKey(range: StatsRange, from: number, to: number): readonly [string, string] {
  return swrKeyStats(range, from, to);
}

export function useStatsPolling(
  range: StatsRange,
  from: number,
  to: number,
  page: number,
  pageSize: number,
  refreshMs: number | null,
  load: (range: StatsRange, from: number, to: number, page: number, pageSize: number, keepOnError: boolean) => Promise<void>,
  windowBounds: () => { from: number; to: number },
) {
  useEffect(() => {
    if (refreshMs == null) return;
    const id = setInterval(() => {
      const { from: f, to: t } = windowBounds();
      void load(range, f, t, page, pageSize, true);
    }, refreshMs);
    return () => clearInterval(id);
  }, [refreshMs, range, from, to, page, pageSize, load, windowBounds]);
}

// Helper to trigger SWR revalidation for stats window (window透传)
export async function mutateStats(range: StatsRange, from: number, to: number): Promise<void> {
  const key = swrKeyStats(range, from, to);
  await mutate(key as unknown as string);
}

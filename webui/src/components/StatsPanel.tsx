import { useEffect, useState } from "react";
import type { AppState } from "../types";
import { UsageStatsSection } from "./UsageStatsSection";

export function StatsPanel({ state }: { state: AppState }) {
  const [refreshMs, setRefreshMs] = useState<number | null>(null);
  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    if (refreshMs == null) return;
    const id = setInterval(() => setRefreshTick((tick) => tick + 1), refreshMs);
    return () => clearInterval(id);
  }, [refreshMs]);

  return (
    <UsageStatsSection
      state={state}
      refreshMs={refreshMs}
      onRefreshMsChange={setRefreshMs}
      refreshTick={refreshTick}
    />
  );
}

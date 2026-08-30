import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import { isCreditsSupported } from "../lib/credits";
import type { NormalizedCredits } from "../lib/credits";
import type { ProviderProfile } from "../types";

export function SupplierCreditsPanel({ name, profile }: { name: string; profile: ProviderProfile }) {
  const [data, setData] = useState<NormalizedCredits | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const supported = isCreditsSupported(profile);

  const fetchCredits = useCallback(async () => {
    if (!supported) return;
    setLoading(true);
    setError(null);
    try {
      const res = await api.getCredits(name);
      setData(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [name, supported]);

  useEffect(() => {
    if (!supported) return;
    void fetchCredits();
  }, [supported, fetchCredits]);

  if (!supported) return null;

  const hasUsage = Boolean(data?.usage && (data.usage.rolling || data.usage.weekly || data.usage.monthly));
  const expiryText = data?.expiry ?? data?.resetAt ?? null;

  const renderWindow = (label: string, win?: { percent: number; status: string; resetsAt?: string | null } | null) => {
    if (!win) return null;
    const pct = Math.max(0, Math.min(100, win.percent));
    const isLimited = win.status === "rate-limited";
    const barColor = isLimited ? "bg-red-500" : pct >= 80 ? "bg-amber-500" : "bg-emerald-500";
    return (
      <div key={label} className="space-y-1">
        <div className="flex items-center justify-between">
          <span className="text-zinc-400">{label}</span>
          <span className="flex items-center gap-1 text-zinc-300">
            <span className="font-medium">{win.percent.toFixed(1)}%</span>
            <span className={isLimited ? "text-red-400" : "text-zinc-500"}>· {win.status}</span>
          </span>
        </div>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
          <div
            data-testid="credits-progress-bar"
            className={`h-1.5 rounded-full transition-all ${barColor}`}
            style={{ width: `${pct}%` }}
          />
        </div>
        {win.resetsAt ? <div className="text-right text-[11px] text-zinc-500">重置 {win.resetsAt}</div> : null}
      </div>
    );
  };

  return (
    <div className="mt-2 rounded-lg border border-white/10 bg-zinc-900/40 p-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-zinc-300">
          {hasUsage ? "用量" : "余量"} <span className="font-normal text-zinc-500">· 主上游</span>
        </span>
        <button
          onClick={() => void fetchCredits()}
          disabled={loading}
          className="inline-flex items-center gap-1 rounded-md border border-white/10 bg-white/[0.06] px-2 py-1 text-xs text-zinc-200 hover:bg-white/[0.10] disabled:opacity-40"
          aria-label="刷新余量"
        >
          {loading ? (
            <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-zinc-500 border-t-zinc-200" aria-hidden="true" />
          ) : null}
          刷新
        </button>
      </div>

      {loading && !data && !error ? (
        <div className="mt-2 flex items-center gap-2 text-xs text-zinc-400">
          <span data-testid="credits-spinner" className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-zinc-500 border-t-zinc-200" aria-label="加载中" />
          加载中…
        </div>
      ) : null}

      {loading && data ? (
        <div className="mt-1 flex items-center gap-2 text-xs text-zinc-500">
          <span data-testid="credits-spinner" className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-zinc-500 border-t-zinc-200" aria-label="加载中" />
          刷新中…
        </div>
      ) : null}

      {error ? (
        <div className="mt-2 flex items-center gap-2">
          <span className="text-xs text-red-400">{error}</span>
          <button
            onClick={() => void fetchCredits()}
            className="rounded-md border border-red-500/30 bg-red-500/15 px-2 py-0.5 text-xs text-red-300 hover:bg-red-500/25"
          >
            重试
          </button>
        </div>
      ) : null}

      {data && !error ? (
        hasUsage ? (
          <div className="mt-2 space-y-2 text-xs">
            {renderWindow("5小时", data.usage?.rolling)}
            {renderWindow("每周", data.usage?.weekly)}
            {renderWindow("每月", data.usage?.monthly)}
          </div>
        ) : (
          <>
            <div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
              <div className="flex items-baseline gap-1">
                <span className="text-zinc-500">余额</span>
                <span className="font-medium text-zinc-100">{data.balance}</span>
              </div>
              <div className="flex items-baseline gap-1">
                <span className="text-zinc-500">总额</span>
                <span className="font-medium text-zinc-100">{data.total}</span>
              </div>
              <div className="flex items-baseline gap-1">
                <span className="text-zinc-500">已用</span>
                <span className="text-zinc-300">{data.used}</span>
              </div>
              <div className="flex items-baseline gap-1">
                <span className="text-zinc-500">过期</span>
                <span className="text-zinc-300 truncate">{expiryText ?? "-"}</span>
              </div>
            </div>
            <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
              <div
                data-testid="credits-progress-bar"
                className="h-1.5 rounded-full bg-amber-500 transition-all"
                style={{ width: `${Math.max(0, Math.min(100, data.percent))}%` }}
              />
            </div>
            <div className="mt-1 text-right text-[11px] text-zinc-500">{data.percent.toFixed(1)}%</div>
          </>
        )
      ) : null}
    </div>
  );
}

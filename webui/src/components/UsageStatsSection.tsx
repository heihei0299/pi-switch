import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AppState, UsageStats } from "../types";
import { api, logsExportUrl } from "../api";
import { Button, Card, Input, SectionTitle, cx } from "./ui";
import { formatCost, formatRequestToken, formatTokenDimension, formatTotalTokens } from "../lib/format";
import { computeStatsWindow, todayString } from "../lib/statsWindow";
import type { StatsRange } from "../lib/statsWindow";
import { useI18n } from "../i18n";
import { PAGE_SIZES, pageNumbers, RequestRow } from "./StatsTableParts";
import { ConversationBrowser } from "./ConversationBrowser";

const REFRESH_TIERS: { label: string; ms: number | null }[] = [
  { label: "Off", ms: null },
  { label: "5s", ms: 5000 },
  { label: "30s", ms: 30_000 },
  { label: "5min", ms: 300_000 },
];

export function UsageStatsSection({
  state,
  refreshMs,
  onRefreshMsChange,
  refreshTick,
}: {
  state: AppState;
  refreshMs: number | null;
  onRefreshMsChange: (value: number | null) => void;
  refreshTick: number;
}) {
  const { t } = useI18n();
  const [stats, setStats] = useState<UsageStats | null>(null);
  const [statsError, setStatsError] = useState<string | null>(null);
  const [range, setRange] = useState<StatsRange>("today");
  const [customFrom, setCustomFrom] = useState("");
  const [customTo, setCustomTo] = useState("");
  const [customError, setCustomError] = useState<string | null>(null);
  const [requestsOpen, setRequestsOpen] = useState(true);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const seq = useRef(0);
  const load = useCallback(
    async (
      range: StatsRange,
      from: number,
      to: number,
      page: number,
      pageSize: number,
      keepOnError = false,
    ) => {
      const id = ++seq.current;
      try {
        const next = await api.stats(range, from, to, page, pageSize);
        if (id === seq.current) {
          const lastPage =
            next.recentRequestTotal != null && next.recentRequestTotal > 0
              ? Math.ceil(next.recentRequestTotal / pageSize) - 1
              : 0;
          if (page > lastPage) {
            // The rolling window shrank the page count while we were on a later
            // page: clamp to the last valid page and re-request (guarded so a
            // clamped page can never re-trigger the clamp).
            setPage(lastPage);
            void load(range, from, to, lastPage, pageSize, keepOnError);
            return;
          }
          setStats(next);
          setStatsError(null);
        }
      } catch (error) {
        // Keep the last successful snapshot, but surface the contract/network
        // error instead of silently rendering an empty dashboard.
        if (id === seq.current) {
          setStatsError(error instanceof Error ? error.message : String(error));
        }
      }
    },
    [],
  );

  useEffect(() => {
    const { from, to } = computeStatsWindow("today", null, null);
    void load("today", from, to, 0, 50);
  }, [load]);

  // Current window bounds for the active range; custom falls back to today.
  const windowBounds = useCallback(
    () =>
      range === "custom"
        ? computeStatsWindow("custom", customFrom || todayString(), customTo || todayString())
        : computeStatsWindow(range, null, null),
    [range, customFrom, customTo],
  );

  useEffect(() => {
    if (refreshTick === 0) return;
    const { from, to } = windowBounds();
    void load(range, from, to, page, pageSize, true);
  }, [refreshTick, range, customFrom, customTo, page, pageSize, load, windowBounds]);
  const select = (key: StatsRange, keepPage = false) => {
    setRange(key);
    if (key === "custom") {
      const from = customFrom || todayString();
      const to = customTo || todayString();
      if (customFrom && customTo && to < from) {
        setCustomError(t("End must be on or after start"));
        return;
      }
      setCustomFrom(from);
      setCustomTo(to);
      setPage(0);
      const { from: f, to: toMs } = computeStatsWindow("custom", from, to);
      void load("custom", f, toMs, 0, pageSize);
    } else {
      setCustomError(null);
      const { from, to } = computeStatsWindow(key, null, null);
      if (!keepPage) {
        setPage(0);
      }
      void load(key, from, to, keepPage ? page : 0, pageSize);
    }
  };

  const onCustomDate =
    (which: "from" | "to") => (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      const from = which === "from" ? value : customFrom;
      const to = which === "to" ? value : customTo;
      if (which === "from") {
        setCustomFrom(value);
      } else {
        setCustomTo(value);
      }
      if (!from || !to) {
        setCustomError(t("Select both start and end dates"));
      } else if (to < from) {
        setCustomError(t("End must be on or after start"));
      } else {
        setCustomError(null);
        setPage(0);
        const { from: f, to: toMs } = computeStatsWindow("custom", from, to);
        void load("custom", f, toMs, 0, pageSize);
      }
    };

  const PRESETS: { key: StatsRange; label: string }[] = [
    { key: "today", label: t("Today") },
    { key: "last24h", label: t("24h") },
    { key: "last7d", label: t("7d") },
    { key: "custom", label: t("Custom") },
  ];

  const byProvider = stats?.byProvider ? Object.entries(stats.byProvider) : [];
  const byModel = stats?.byModel ? Object.entries(stats.byModel) : [];
  const totals = stats?.totalTokens;
  const totalRows = stats?.recentRequestTotal;
  const totalPages = totalRows != null && totalRows > 0 ? Math.ceil(totalRows / pageSize) : 0;
  const goPage = (nextPage: number) => {
    setPage(nextPage);
    const { from, to } = windowBounds();
    void load(range, from, to, nextPage, pageSize);
  };
  return (
    <div>
      <SectionTitle hint={t("proxy request usage")}>{t("Stats")}</SectionTitle>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        {PRESETS.map(({ key, label }) => (
          <Button
            key={key}
            variant={range === key ? "primary" : "subtle"}
            aria-pressed={range === key}
            onClick={() => select(key)}
          >
            {label}
          </Button>
        ))}
        {range === "custom" && (
          <span className="flex flex-wrap items-center gap-2">
            <Input type="date" aria-label={t("From")} value={customFrom} onChange={onCustomDate("from")} className="!w-auto" />
            <span className="text-xs text-zinc-500">→</span>
            <Input type="date" aria-label={t("To")} value={customTo} onChange={onCustomDate("to")} className="!w-auto" />
            {customError && <span className="text-xs text-red-300">{customError}</span>}
          </span>
        )}
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Button onClick={() => select(range, true)}>{t("Refresh")}</Button>
        <label className="flex items-center gap-1 text-xs text-zinc-500">
          Auto-refresh
          <select
            aria-label={t("Auto-refresh")}
            value={refreshMs ?? "off"}
            onChange={(e) => onRefreshMsChange(e.target.value === "off" ? null : Number(e.target.value))}
            className="rounded border border-white/10 bg-zinc-900 px-1.5 py-0.5 text-xs text-zinc-200"
          >
            {REFRESH_TIERS.map(({ label, ms }) => (
              <option key={label} value={ms == null ? "off" : String(ms)}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <a href={logsExportUrl("json")} className="inline-flex">
          <Button>{t("Export JSON")}</Button>
        </a>
        <a href={logsExportUrl("csv")} className="inline-flex">
          <Button>{t("Export CSV")}</Button>
        </a>
      </div>

      {statsError && (
        <Card className="mb-3 border-red-500/30">
          <div role="alert" className="break-words text-sm text-red-300">{statsError}</div>
          {stats && <div className="mt-1 text-xs text-zinc-500">Showing the last successful snapshot.</div>}
        </Card>
      )}

      {!stats || stats.totalRequests === 0 ? (
        <Card>
          <div className={statsError ? "break-words text-sm text-red-300" : "text-sm text-zinc-500"}>
            {statsError ?? t("No request data yet. Start the proxy and make some requests.")}
          </div>
        </Card>
      ) : (
        <>
          <div className="mb-4 rounded-xl border border-line bg-panel/50 p-3">
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
              <HeroMetric label={t("Total")} value={String(stats.totalRequests)} accent />
              <HeroMetric label={t("OK")} value={String(stats.okRequests)} tone="green" />
              <HeroMetric label={t("Failed")} value={String(stats.failedRequests)} tone="red" />
              <HeroMetric label={t("Success")} value={String(stats.successRate)} />
              <HeroMetric label={t("Cache rate")} value={String(stats.cacheHitRate ?? "-")} />
            </div>
            <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
              <HeroMetric label={t("Input")} value={String(formatTokenDimension(totals?.input))} mono />
              <HeroMetric label={t("Output")} value={String(formatTokenDimension(totals?.output))} mono />
              <HeroMetric label={t("Cached")} value={String(formatTokenDimension(totals?.cached))} mono badge="⊆ Input" />
              <HeroMetric label={t("Reasoning")} value={String(formatTokenDimension(totals?.reasoning))} mono badge="⊆ Output" />
              <HeroMetric label={t("Total")} value={String(formatTotalTokens(totals))} mono accent />
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-3 border-t border-white/5 pt-3">
              <HeroMetric label="Cost" value={String(formatCost(stats.totalCost))} mono accent />
              {stats.costUnknown ? (
                <span className="text-xs tracking-wide text-zinc-500">
                  {stats.costUnknown} {t("unknown cost rows")}
                </span>
              ) : null}
              {stats.avgLatencyMs != null && (
                <span className="text-xs text-zinc-500">
                  {t("Avg latency:")} <span className="font-mono text-zinc-200">{stats.avgLatencyMs} ms</span>
                </span>
              )}
            </div>
          </div>

          {byProvider.length > 0 && (
            <Card className="overflow-hidden">
              <div className="mb-2 text-sm font-semibold text-zinc-200">{t("By provider")}</div>
              <div className="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
                <table aria-label={t("By provider")} className="w-full min-w-[480px] text-sm sm:min-w-[640px]">
                  <thead className="text-left text-xs text-zinc-500">
                    <tr>
                      <th className="sticky left-0 z-10 bg-transparent pb-1 pr-2">{t("Provider")}</th>
                      <th className="pb-1 text-right">{t("Requests")}</th>
                      <th className="pb-1 text-right">{t("OK")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Rate")}</th>
                      <th className="pb-1 text-right">{t("Input")}</th>
                      <th className="pb-1 text-right">{t("Output")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Cached")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Total")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Cache rate")}</th>
                      <th className="pb-1 text-right">{t("Cost")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {byProvider.map(([name, ps]) => {
                      const rate = ps.total > 0 ? Math.round((ps.ok / ps.total) * 100) : 0;
                      return (
                        <tr key={name} className="border-t border-white/5">
                          <td className="sticky left-0 z-10 bg-transparent py-1 pr-2 text-zinc-200">{name}</td>
                          <td className="py-1 text-right text-zinc-400">{ps.total}</td>
                          <td className="py-1 text-right text-zinc-400">{ps.ok}</td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">{rate}%</td>
                          <td className="py-1 text-right text-zinc-400">
                            {formatRequestToken(ps.promptTokens)}
                          </td>
                          <td className="py-1 text-right text-zinc-400">
                            {formatRequestToken(ps.outputTokens)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {formatRequestToken(ps.cachedTokens)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {formatRequestToken(ps.promptTokens + ps.outputTokens)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {ps.cacheRate ?? "-"}
                          </td>
                          <td className="py-1 text-right text-zinc-400">{formatCost(ps.cost)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </Card>
          )}

          {byModel.length > 0 && (
            <Card className="mt-4 overflow-hidden">
              <div className="mb-2 text-sm font-semibold text-zinc-200">{t("By model")}</div>
              <div className="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
                <table aria-label={t("By model")} className="w-full min-w-[480px] text-sm sm:min-w-[640px]">
                  <thead className="text-left text-xs text-zinc-500">
                    <tr>
                      <th className="sticky left-0 z-10 bg-transparent pb-1 pr-2">{t("Model")}</th>
                      <th className="pb-1 text-right">{t("Requests")}</th>
                      <th className="pb-1 text-right">{t("OK")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Rate")}</th>
                      <th className="pb-1 text-right">{t("Input")}</th>
                      <th className="pb-1 text-right">{t("Output")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Cached")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Total")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Cache rate")}</th>
                      <th className="pb-1 text-right">{t("Cost")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {byModel.map(([name, ms]) => {
                      const input = ms.promptTokens ?? 0;
                      const output = ms.outputTokens ?? 0;
                      const rate = ms.total > 0 ? Math.round((ms.ok / ms.total) * 100) : 0;
                      return (
                        <tr key={name} className="border-t border-white/5">
                          <td className="sticky left-0 z-10 max-w-[10rem] truncate bg-transparent py-1 pr-2 text-zinc-200" title={name}>
                            {name}
                          </td>
                          <td className="py-1 text-right text-zinc-400">{ms.total}</td>
                          <td className="py-1 text-right text-zinc-400">{ms.ok}</td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">{rate}%</td>
                          <td className="py-1 text-right text-zinc-400">
                            {formatRequestToken(input)}
                          </td>
                          <td className="py-1 text-right text-zinc-400">
                            {formatRequestToken(output)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {formatRequestToken(ms.cachedTokens ?? 0)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {formatRequestToken(input + output)}
                          </td>
                          <td className="hidden py-1 text-right text-zinc-400 sm:table-cell">
                            {ms.cacheRate ?? "-"}
                          </td>
                          <td className="py-1 text-right text-zinc-400">{formatCost(ms.cost)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </Card>
          )}

          {stats.recentRequests?.length ? (
            <Card className="mt-4 overflow-hidden">
              <button
                type="button"
                aria-expanded={requestsOpen}
                onClick={() => setRequestsOpen((v) => !v)}
                className="mb-2 flex w-full items-center justify-between text-sm font-semibold text-zinc-200"
              >
                <span>{t("Request details")}</span>
                <span className="text-zinc-500">{requestsOpen ? "▾" : "▸"}</span>
              </button>
              {requestsOpen && (
                <>
                  <div className="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
                <table aria-label={t("Request details")} className="w-full min-w-[560px] text-sm sm:min-w-[760px]">
                  <thead className="text-left text-xs text-zinc-500">
                    <tr>
                      <th className="sticky left-0 z-10 bg-transparent pb-1 pr-2">{t("Time")}</th>
                      <th className="pb-1 pr-2">{t("Session")}</th>
                      <th className="pb-1 pr-2">{t("Provider")}</th>
                      <th className="pb-1 pr-2">{t("Model")}</th>
                      <th className="pb-1 pr-2">{t("Status")}</th>
                      <th className="pb-1 pr-2 text-right">{t("Input")}</th>
                      <th className="pb-1 pr-2 text-right">{t("Output")}</th>
                      <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Cached")}</th>
                      <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Reasoning")}</th>
                      <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Cache rate")}</th>
                      <th className="hidden pb-1 text-right sm:table-cell">{t("Total")}</th>
                      <th className="pb-1 text-right">{t("Cost")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.recentRequests.map((r, i) => (
                      <RequestRow key={`${r.ts ?? ""}-${r.model ?? ""}-${i}`} r={r} i={i} />
                    ))}
                  </tbody>
                </table>
              </div>
              {totalRows != null && totalRows > 0 && (
                <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-400">
                  <span>{totalRows} {t("rows")}</span>
                  {totalPages > 1 && (
                    <span className="flex items-center gap-1">
                      <Button
                        aria-label={t("Previous page")}
                        disabled={page === 0}
                        onClick={() => goPage(page - 1)}
                      >
                        ‹
                      </Button>
                      {pageNumbers(page, totalPages).map((n, i) =>
                        n === "…" ? (
                          <span key={`gap-${i}`} className="px-1 text-zinc-600">
                            …
                          </span>
                        ) : (
                          <Button
                            key={n}
                            variant={n - 1 === page ? "primary" : "subtle"}
                            aria-pressed={n - 1 === page}
                            onClick={() => goPage(n - 1)}
                          >
                            {n}
                          </Button>
                        ),
                      )}
                      <Button
                        aria-label={t("Next page")}
                        disabled={page >= totalPages - 1}
                        onClick={() => goPage(page + 1)}
                      >
                        ›
                      </Button>
                    </span>
                  )}
                  <label className="flex items-center gap-1 text-zinc-500">
                    Rows per page
                    <select
                      aria-label={t("Rows per page")}
                      value={pageSize}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setPageSize(next);
                        setPage(0);
                        const { from, to } = windowBounds();
                        void load(range, from, to, 0, next);
                      }}
                      className="rounded border border-white/10 bg-zinc-900 px-1.5 py-0.5 text-xs text-zinc-200"
                    >
                      {PAGE_SIZES.map((s) => (
                        <option key={s} value={s}>
                          {s}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              )}
                </>
              )}
            </Card>
          ) : null}

        </>
      )}
      <ConversationBrowser
        visible={Boolean(stats && stats.totalRequests > 0 && state.settings.conversationSource !== "off")}
        refreshTick={refreshTick}
      />
    </div>
  );
}

function HeroMetric({
  label,
  value,
  tone = "zinc",
  badge,
  mono,
  accent,
}: {
  label: string;
  value: string;
  tone?: "zinc" | "green" | "red";
  badge?: string;
  mono?: boolean;
  accent?: boolean;
}) {
  const color = tone === "green" ? "text-emerald-300" : tone === "red" ? "text-red-300" : accent ? "text-amber-200" : "text-zinc-100";
  return (
    <div className={cx("rounded-lg border bg-zinc-950/40 px-3 py-2.5", accent ? "border-amber-500/20 bg-amber-500/[0.06]" : "border-white/5")}>
      <div className="text-[10px] font-semibold uppercase tracking-widest text-zinc-500">
        {label}
        {badge && <span className="ml-1 text-[9px] normal-case tracking-normal text-zinc-600">{badge}</span>}
      </div>
      <div className={cx("mt-1 text-[17px] font-semibold leading-none tracking-tight", mono ? "font-mono tabular-nums" : "font-display", color)}>{value}</div>
    </div>
  );
}

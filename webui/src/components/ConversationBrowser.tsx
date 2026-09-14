import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import type { ConversationRequestsPage, ConversationStats, ConversationsPage } from "../types";
import { api } from "../api";
import { decodeConversationName, formatCost, formatRequestTime, formatRequestToken, isLowCacheRate, shortConversationId } from "../lib/format";
import { computeConversationWindow, todayString } from "../lib/statsWindow";
import type { ConversationRange } from "../lib/statsWindow";
import { useI18n } from "../i18n";
import { Button, Card, Input } from "./ui";
import { PAGE_SIZES, CopyableSessionCell, pageNumbers, RequestRow } from "./StatsTableParts";

const CONV_PRESETS: { key: ConversationRange; label: string }[] = [
  { key: "today", label: "Today" },
  { key: "last24h", label: "24h" },
  { key: "last7d", label: "7d" },
  { key: "custom", label: "Custom" },
  { key: "all", label: "All-time" },
];

export function ConversationBrowser({ refreshTick, visible }: { refreshTick: number; visible: boolean }) {
  const { t } = useI18n();
  const [conversationsOpen, setConversationsOpen] = useState(false);
  const [convRange, setConvRange] = useState<ConversationRange>("today");
  const [convFrom, setConvFrom] = useState("");
  const [convTo, setConvTo] = useState("");
  const [convError, setConvError] = useState<string | null>(null);
  const [convLoadError, setConvLoadError] = useState<string | null>(null);
  const [convPage, setConvPage] = useState(0);
  const [convPageSize, setConvPageSize] = useState(50);
  const [convData, setConvData] = useState<ConversationsPage | null>(null);
  const [expandedConvs, setExpandedConvs] = useState<Set<string>>(new Set());
  const convSeq = useRef(0);
  const loadConversations = useCallback(
    async (
      range: ConversationRange,
      from: number | null,
      to: number | null,
      page: number,
      pageSize: number,
      keepOnError = false,
    ) => {
      const id = ++convSeq.current;
      try {
        const next = await api.statsConversations(range, from, to, page, pageSize);
        if (id === convSeq.current) {
          const lastPage = next.total > 0 ? Math.ceil(next.total / pageSize) - 1 : 0;
          if (page > lastPage) {
            // Same clamp semantics as the main load: a shrunken window can
            // invalidate the current page, so clamp and re-request once.
            setConvPage(lastPage);
            void loadConversations(range, from, to, lastPage, pageSize, keepOnError);
            return;
          }
          setConvData(next);
          setConvLoadError(null);
        }
      } catch (error) {
        if (id === convSeq.current) {
          setConvLoadError(error instanceof Error ? error.message : String(error));
        }
      }
    },
    [],
  );

  useEffect(() => {
    if (!conversationsOpen || convData != null) {
      return;
    }
    const conv = computeConversationWindow("today", null, null);
    void loadConversations("today", conv.from, conv.to, 0, 50);
  }, [conversationsOpen, convData, loadConversations]);

  // Independent window bounds for the conversation browser; "all" is a null
  // window (full history).
  const convWindowBounds = useCallback(
    () =>
      convRange === "custom"
        ? computeConversationWindow("custom", convFrom || todayString(), convTo || todayString())
        : computeConversationWindow(convRange, null, null),
    [convRange, convFrom, convTo],
  );

  useEffect(() => {
    if (refreshTick === 0 || !conversationsOpen) return;
    const conv = convWindowBounds();
    void loadConversations(convRange, conv.from, conv.to, convPage, convPageSize, true);
  }, [refreshTick, conversationsOpen, convRange, convFrom, convTo, convPage, convPageSize, loadConversations, convWindowBounds]);
  const convSelect = (key: ConversationRange) => {
    setConvRange(key);
    if (key === "custom") {
      const from = convFrom || todayString();
      const to = convTo || todayString();
      if (convFrom && convTo && to < from) {
        setConvError("End must be on or after start");
        return;
      }
      setConvFrom(from);
      setConvTo(to);
      setConvPage(0);
      const { from: f, to: t } = computeConversationWindow("custom", from, to);
      void loadConversations("custom", f, t, 0, convPageSize);
    } else {
      setConvError(null);
      const { from, to } = computeConversationWindow(key, null, null);
      setConvPage(0);
      void loadConversations(key, from, to, 0, convPageSize);
    }
  };

  const onConvCustomDate =
    (which: "from" | "to") => (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      const from = which === "from" ? value : convFrom;
      const to = which === "to" ? value : convTo;
      if (which === "from") {
        setConvFrom(value);
      } else {
        setConvTo(value);
      }
      if (!from || !to) {
        setConvError("Select both start and end dates");
      } else if (to < from) {
        setConvError("End must be on or after start");
      } else {
        setConvError(null);
        setConvPage(0);
        const { from: f, to: toMs } = computeConversationWindow("custom", from, to);
        void loadConversations("custom", f, toMs, 0, convPageSize);
      }
    };

  const convTotalPages =
    convData && convData.total > 0 ? Math.ceil(convData.total / convPageSize) : 0;
  const convGoPage = (nextPage: number) => {
    setConvPage(nextPage);
    const { from, to } = convWindowBounds();
    void loadConversations(convRange, from, to, nextPage, convPageSize);
  };
  const toggleConv = (id: string) => {
    setExpandedConvs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  return visible ? (
    <>
          <Card className="mt-4 overflow-hidden">
            <button
              type="button"
              aria-expanded={conversationsOpen}
              onClick={() => setConversationsOpen((v) => !v)}
              className="mb-2 flex w-full items-center justify-between text-sm font-semibold text-zinc-200"
            >
              <span>{t("By conversation")}</span>
              <span className="text-zinc-500">{conversationsOpen ? "▾" : "▸"}</span>
            </button>
            {conversationsOpen && (
              <div>
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  {CONV_PRESETS.map(({ key, label }) => (
                    <Button
                      key={key}
                      variant={convRange === key ? "primary" : "subtle"}
                      aria-pressed={convRange === key}
                      onClick={() => convSelect(key)}
                    >
                      {label}
                    </Button>
                  ))}
                  {convRange === "custom" && (
                    <span className="flex flex-wrap items-center gap-2">
                      <Input type="date" aria-label={t("Conversation from")} value={convFrom} onChange={onConvCustomDate("from")} className="!w-auto" />
                      <span className="text-xs text-zinc-500">→</span>
                      <Input type="date" aria-label={t("Conversation to")} value={convTo} onChange={onConvCustomDate("to")} className="!w-auto" />
                      {convError && <span className="text-xs text-red-300">{convError}</span>}
                    </span>
                  )}
                </div>
                {convLoadError ? (
                  <div role="alert" className="break-words text-sm text-red-300">{convLoadError}</div>
                ) : !convData || convData.total === 0 ? (
                  <div className="text-sm text-zinc-500">{t("No conversation data in this range.")}</div>
                ) : (
                  <>
                    <div className="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
                      <table aria-label={t("By conversation")} className="w-full min-w-[520px] text-sm sm:min-w-[760px]">
                        <thead className="text-left text-xs text-zinc-500">
                          <tr>
                            <th className="pb-1 pr-2"></th>
                            <th className="sticky left-0 z-10 bg-transparent pb-1 pr-2">{t("Time")}</th>
                            <th className="pb-1 pr-2">{t("Session")}</th>
                            <th className="pb-1 pr-2 text-right">{t("Requests")}</th>
                            <th className="pb-1 pr-2 text-right">{t("Input")}</th>
                            <th className="pb-1 pr-2 text-right">{t("Output")}</th>
                            <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Cached")}</th>
                            <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Reasoning")}</th>
                            <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Cache rate")}</th>
                            <th className="hidden pb-1 pr-2 text-right sm:table-cell">{t("Total")}</th>
                            <th className="pb-1 text-right">{t("Cost")}</th>
                          </tr>
                        </thead>
                        <tbody>
                          {convData.conversations.map((c) => (
                            <Fragment key={c.conversationId}>
                              <tr className="border-t border-white/5">
                                <td className="py-1 pr-2">
                                  <button
                                    type="button"
                                    aria-expanded={expandedConvs.has(c.conversationId)}
                                    aria-label={`Expand conversation ${c.conversationId}`}
                                    onClick={() => toggleConv(c.conversationId)}
                                    className="text-zinc-500 hover:text-zinc-200"
                                  >
                                    {expandedConvs.has(c.conversationId) ? "▾" : "▸"}
                                  </button>
                                </td>
                                <td className="sticky left-0 z-10 bg-transparent py-1 pr-2 whitespace-nowrap text-zinc-500">
                                  {formatRequestTime(c.lastActive)}
                                </td>
                                <td className="py-1 pr-2">
                                  <CopyableSessionCell id={c.conversationId} name={c.name} className="max-w-[14rem]" />
                                </td>
                                <td className="py-1 pr-2 text-right text-zinc-400">{c.requests}</td>
                                <td className="py-1 pr-2 text-right text-zinc-400">
                                  {formatRequestToken(c.inputTokens)}
                                </td>
                                <td className="py-1 pr-2 text-right text-zinc-400">
                                  {formatRequestToken(c.outputTokens)}
                                </td>
                                <td className="hidden py-1 pr-2 text-right text-zinc-400 sm:table-cell">
                                  {formatRequestToken(c.cachedTokens)}
                                </td>
                                <td className="hidden py-1 pr-2 text-right text-zinc-400 sm:table-cell">
                                  {formatRequestToken(c.reasoningTokens)}
                                </td>
                                <td className={`hidden py-1 pr-2 text-right sm:table-cell ${isLowCacheRate(c.cacheRate) ? "text-red-300" : "text-zinc-400"}`}>{c.cacheRate ?? "-"}</td>
                                <td className="hidden py-1 pr-2 text-right text-zinc-400 sm:table-cell">
                                  {formatRequestToken(c.inputTokens + c.outputTokens)}
                                </td>
                                <td className="py-1 text-right text-zinc-400">{formatCost(c.cost)}</td>
                              </tr>
                              {expandedConvs.has(c.conversationId) && (
                                <tr className="border-t border-white/5">
                                  <td colSpan={11} className="py-2 pl-8 pr-2">
                                    <ExpandedConversationRequests conv={c} />
                                  </td>
                                </tr>
                              )}
                            </Fragment>
                          ))}
                        </tbody>
                      </table>
                    </div>
                    <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-400">
                      <span>{convData.total} {t("rows")}</span>
                      {convTotalPages > 1 && (
                        <span className="flex items-center gap-1">
                          <Button
                            aria-label={t("Previous conversation page")}
                            disabled={convPage === 0}
                            onClick={() => convGoPage(convPage - 1)}
                          >
                            ‹
                          </Button>
                          {pageNumbers(convPage, convTotalPages).map((n, i) =>
                            n === "…" ? (
                              <span key={`gap-${i}`} className="px-1 text-zinc-600">
                                …
                              </span>
                            ) : (
                              <Button
                                key={n}
                                variant={n - 1 === convPage ? "primary" : "subtle"}
                                aria-pressed={n - 1 === convPage}
                                onClick={() => convGoPage(n - 1)}
                              >
                                {n}
                              </Button>
                            ),
                          )}
                          <Button
                            aria-label={t("Next conversation page")}
                            disabled={convPage >= convTotalPages - 1}
                            onClick={() => convGoPage(convPage + 1)}
                          >
                            ›
                          </Button>
                        </span>
                      )}
                      <label className="flex items-center gap-1 text-zinc-500">
                        Rows per page
                        <select
                          aria-label={t("Conversation rows per page")}
                          value={convPageSize}
                          onChange={(e) => {
                            const next = Number(e.target.value);
                            setConvPageSize(next);
                            setConvPage(0);
                            const { from, to } = convWindowBounds();
                            void loadConversations(convRange, from, to, 0, next);
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
                  </>
                )}
              </div>
            )}
          </Card>
    </>
  ) : null;
}

// ─── Expanded conversation request browser ────────────────────

function ExpandedConversationRequests({ conv }: { conv: ConversationStats }) {
  const { t } = useI18n();
  const [data, setData] = useState<ConversationRequestsPage | null>(null);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [error, setError] = useState(false);
  const seq = useRef(0);

  const load = useCallback(
    async (page: number, pageSize: number) => {
      const id = ++seq.current;
      setError(false);
      try {
        const next = await api.conversationRequests(conv.conversationId, page, pageSize);
        if (id === seq.current) {
          const lastPage = next.total > 0 ? Math.ceil(next.total / pageSize) - 1 : 0;
          if (page > lastPage) {
            // The conversation shrank while we were on a later page: clamp
            // and re-request once (guarded so it can never re-trigger).
            setPage(lastPage);
            void load(lastPage, pageSize);
            return;
          }
          setData(next);
        }
      } catch {
        if (id === seq.current) {
          setError(true);
        }
      }
    },
    [conv.conversationId],
  );

  useEffect(() => {
    void load(0, 50);
  }, [load]);

  const totalPages = data && data.total > 0 ? Math.ceil(data.total / pageSize) : 0;
  const goPage = (nextPage: number) => {
    setPage(nextPage);
    void load(nextPage, pageSize);
  };

  return (
    <div>
      <div className="mb-1 text-xs text-zinc-500">
        Requests in {decodeConversationName(conv.name ?? "") || shortConversationId(conv.conversationId)}
      </div>
      {error ? (
        <div className="text-sm text-red-300">Failed to load conversation requests.</div>
      ) : !data ? (
        <div className="text-sm text-zinc-500">Loading…</div>
      ) : data.requests.length === 0 ? (
        <div className="text-sm text-zinc-500">No requests in this conversation.</div>
      ) : (
        <>
          <div className="-mx-2 overflow-x-auto px-2 sm:mx-0 sm:px-0">
            <table aria-label={`Requests of ${conv.conversationId}`} className="w-full min-w-[560px] text-sm sm:min-w-[760px]">
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
                {data.requests.map((r, i) => (
                  <RequestRow key={`${r.ts ?? ""}-${r.model ?? ""}-${i}`} r={r} i={i} />
                ))}
              </tbody>
            </table>
          </div>
          <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-400">
            <span>{data.total} rows</span>
            {totalPages > 1 && (
              <span className="flex items-center gap-1">
                <Button
                  aria-label="Previous request page"
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
                  aria-label="Next request page"
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
                aria-label="Request rows per page"
                value={pageSize}
                onChange={(e) => {
                  const next = Number(e.target.value);
                  setPageSize(next);
                  setPage(0);
                  void load(0, next);
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
        </>
      )}
    </div>
  );
}

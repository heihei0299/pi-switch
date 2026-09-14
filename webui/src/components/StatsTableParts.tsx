import { useState } from "react";
import type { RecentRequest } from "../types";
import { decodeConversationName, formatCost, formatRequestTime, formatRequestToken, isLowCacheRate, shortConversationId } from "../lib/format";

export const PAGE_SIZES = [50, 100, 200, 500];

export function pageNumbers(current: number, total: number): (number | "…")[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  const wanted = new Set(
    [1, total, current, current + 1, current + 2].map((p) => Math.min(Math.max(p, 1), total)),
  );
  const sorted = [...wanted].sort((a, b) => a - b);
  const out: (number | "…")[] = [];
  let prev = 0;
  for (const p of sorted) {
    if (p - prev > 1) {
      out.push("…");
    }
    out.push(p);
    prev = p;
  }
  return out;
}

function formatRequestStatus(r: RecentRequest): string {
  if (r.ok) {
    return r.status != null ? String(r.status) : "ok";
  }
  const parts = [r.status != null ? String(r.status) : null, r.error ?? null].filter(Boolean);
  return parts.join(" ") || "failed";
}

// ─── Request detail row (shared by the stats page and the expanded
// ─── conversation browser so both render identically) ─────────

export function RequestRow({ r, i }: { r: RecentRequest; i: number }) {
  const status = formatRequestStatus(r);
  const tokenCols = [
    ["Input", formatRequestToken(r.promptTokens)],
    ["Output", formatRequestToken(r.completionTokens)],
    ["Cached", formatRequestToken(r.cachedTokens)],
    ["Reasoning", formatRequestToken(r.reasoningTokens)],
    ["Cache rate", r.cacheRate ?? "-", isLowCacheRate(r.cacheRate) ? "text-red-300" : undefined],
    ["Total", formatRequestToken(r.totalTokens)],
    ["Cost", formatCost(r.cost)],
  ] as const;
  return (
    <tr className="border-t border-white/5">
      <td className="sticky left-0 z-10 bg-transparent py-1 pr-2 whitespace-nowrap text-zinc-500">
        {formatRequestTime(r.ts)}
      </td>
      <td className="py-1 pr-2">
        <CopyableSessionCell id={r.conversationId} name={r.conversationName} />
      </td>
      <td className="py-1 pr-2 text-zinc-300">{r.provider ?? "-"}</td>
      <td className="py-1 pr-2 text-zinc-300">{r.model ?? "-"}</td>
      <td className="py-1 pr-2 text-zinc-400">
        <span className="block max-w-[14rem] truncate" title={status}>
          {status}
        </span>
      </td>
      {tokenCols.map(([label, value, tone]) => {
        const hide = label === "Cached" || label === "Reasoning" || label === "Cache rate" || label === "Total" ? "hidden sm:table-cell" : "";
        return (
          <td key={label} className={`py-1 pr-2 text-right ${hide} ${tone ?? "text-zinc-400"}`}>
            {value}
          </td>
        );
      })}
    </tr>
  );
}

/**
 * Session cell that shows the display name (or a truncated id) and copies
 * the full conversation id to the clipboard on click.
 */
export function CopyableSessionCell({
  id,
  name,
  className = "",
}: {
  id?: string | null;
  name?: string | null;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  const display = decodeConversationName(name ?? "") || (id ? shortConversationId(id) : "-");
  return (
    <button
      type="button"
      title={id ?? undefined}
      aria-label={id ? `Copy conversation ${id}` : undefined}
      onClick={() => {
        if (!id) return;
        navigator.clipboard
          ?.writeText(id)
          .then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1200);
          })
          .catch(() => {});
      }}
      className={`block max-w-[12rem] truncate text-left text-zinc-300 hover:text-zinc-100 ${className}`}
    >
      {copied ? "✓" : display}
    </button>
  );
}


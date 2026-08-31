import { useState } from "react";
import { Button, Input, Switch } from "./ui";
import { useI18n } from "../i18n";
import {
  PI_THINKING_LEVELS,
  isPiThinkingLevelMap,
  supportsImageInput,
  withImageInput,
  type ModelDraft,
  type PiThinkingLevel,
  type PiThinkingLevelMap,
} from "../lib/piModel";
import type { ModelEntry } from "../types";

export function ModelCard({
  draft,
  exposed,
  onToggleExposed,
  onChange,
  onRemove,
  expanded,
  onToggleExpanded,
}: {
  draft: ModelDraft;
  exposed: boolean;
  onToggleExposed: () => void;
  onChange: (next: ModelDraft) => void;
  onRemove: () => void;
  expanded: boolean;
  onToggleExpanded: () => void;
}) {
  const { t } = useI18n() as any;
  const [costExpanded, setCostExpanded] = useState(false);
  const [thinkingExpanded, setThinkingExpanded] = useState(false);

  const thinkingMap: PiThinkingLevelMap | undefined = isPiThinkingLevelMap(draft.thinkingLevelMap)
    ? (draft.thinkingLevelMap as PiThinkingLevelMap)
    : draft.hasThinkingLevelMap
      ? undefined
      : undefined;
  // If draft has no thinkingLevelMap (hasThinkingLevelMap false) we treat as empty map for editing but not persisted until changed
  const effectiveMap: PiThinkingLevelMap = isPiThinkingLevelMap(draft.thinkingLevelMap)
    ? (draft.thinkingLevelMap as PiThinkingLevelMap)
    : {};

  function update(fields: Partial<ModelDraft>) {
    onChange({ ...draft, ...fields });
  }

  function updateThinkingLevel(level: PiThinkingLevel, mode: "default" | "unsupported" | string) {
    let next: PiThinkingLevelMap = { ...effectiveMap };
    if (mode === "default") {
      delete (next as Record<string, unknown>)[level];
    } else if (mode === "unsupported") {
      (next as Record<string, unknown>)[level] = null;
    } else {
      (next as Record<string, unknown>)[level] = mode;
    }
    // If map becomes empty and original had no map, we may keep hasThinkingLevelMap false to mean use default {}
    // But if user explicitly edited, we set hasThinkingLevelMap true
    onChange({ ...draft, thinkingLevelMap: next, hasThinkingLevelMap: true });
  }

  const tOr = (key: string, fallback: string) => (t(key) !== key ? t(key) : fallback);

  return (
    <div className="rounded-xl border border-white/10 bg-zinc-900/40">
      <div className="flex items-center gap-2 p-2">
        <button
          type="button"
          onClick={onToggleExpanded}
          aria-label={t("Toggle model details")}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-zinc-400 hover:bg-white/5 hover:text-zinc-200"
        >
          <span className={`inline-block text-xs transition-transform ${expanded ? "rotate-90" : ""}`}>›</span>
        </button>

        <label className="flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={exposed}
            onChange={onToggleExposed}
            title={t("Checked = exposed")}
            className="h-3.5 w-3.5 rounded border-white/20 bg-zinc-800"
          />
        </label>

        <div className="flex min-w-0 flex-1 gap-2">
          <Input
            id={`model-id-${draft.key}`}
            value={draft.id}
            onChange={(e) => update({ id: e.target.value })}
            placeholder={t("Model ID")}
            aria-label={t("Model ID")}
            className={`min-w-0 flex-1 ${!draft.id.trim() ? "border-amber-500/40" : ""}`}
          />
          <Input
            value={draft.name}
            onChange={(e) => update({ name: e.target.value, hasName: true })}
            placeholder={t("Display name")}
            aria-label={t("Display name")}
            className="min-w-0 flex-1"
          />
        </div>

        <button
          type="button"
          onClick={onRemove}
          aria-label={t("remove")}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-zinc-500 hover:bg-red-500/10 hover:text-red-300"
        >
          🗑
        </button>
      </div>

      {expanded && (
        <div className="border-t border-white/5 p-3">
          <div className="grid gap-3">
            {/* switches */}
            <div className="flex flex-wrap items-center gap-6">
              <label className="flex items-center gap-2.5 text-sm text-zinc-300">
                <span>{t("Support reasoning")}</span>
                <Switch
                  checked={draft.reasoning}
                  onChange={() => update({ reasoning: !draft.reasoning, hasReasoning: true })}
                />
              </label>
              <label className="flex items-center gap-2.5 text-sm text-zinc-300">
                <span>{t("Support image input")}</span>
                <Switch
                  checked={supportsImageInput(draft.input)}
                  onChange={( ) => {
                    const enabled = !supportsImageInput(draft.input);
                    update({ input: withImageInput(draft.input, enabled), hasInput: true });
                  }}
                />
              </label>
            </div>

            {/* context / maxTokens */}
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <label className="mb-1 block text-xs font-medium text-zinc-400">
                  {t("Context window")} <span className="text-red-400">*</span>
                </label>
                <Input
                  type="number"
                  inputMode="numeric"
                  min={1}
                  value={draft.contextWindow}
                  onChange={(e) => update({ contextWindow: e.target.value, hasContextWindow: true })}
                  placeholder="128000"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium text-zinc-400">
                  {t("Max tokens")} <span className="text-red-400">*</span>
                </label>
                <Input
                  type="number"
                  inputMode="numeric"
                  min={1}
                  value={draft.maxTokens}
                  onChange={(e) => update({ maxTokens: e.target.value, hasMaxTokens: true })}
                  placeholder="16384"
                />
              </div>
            </div>

            {/* thinkingLevelMap */}
            {draft.reasoning && (
              <div className="rounded-lg border border-white/10">
                <button
                  type="button"
                  onClick={() => setThinkingExpanded((v) => !v)}
                  className="flex w-full items-center justify-between px-3 py-2 text-sm text-zinc-200 hover:bg-white/[0.03]"
                >
                  <span className="font-medium">{t("Thinking levels")}</span>
                  <span className="text-xs text-zinc-500">{thinkingExpanded ? "∧" : "∨"}</span>
                </button>
                {thinkingExpanded && (
                  <div className="border-t border-white/10">
                    <div className="divide-y divide-white/5">
                      {PI_THINKING_LEVELS.map((level) => {
                        const hasKey = Object.prototype.hasOwnProperty.call(effectiveMap, level);
                        const raw = hasKey ? (effectiveMap as Record<string, unknown>)[level] : undefined;
                        const mode = !hasKey ? "default" : raw === null ? "unsupported" : String(raw);
                        const display =
                          mode === "default"
                            ? t("Default")
                            : mode === "unsupported"
                              ? t("Unsupported")
                              : mode;
                        return (
                          <div key={level} className="flex items-center justify-between px-3 py-2 text-sm">
                            <span className="capitalize text-zinc-300">{level === "xhigh" ? "XHigh" : level === "off" ? t("Off") : level}</span>
                            <div className="flex items-center gap-2">
                              <span className="text-xs text-zinc-500">{display}</span>
                              <select
                                value={mode === "default" ? "__default__" : mode === "unsupported" ? "__unsupported__" : mode}
                                onChange={(e) => {
                                  const v = e.target.value;
                                  if (v === "__default__") updateThinkingLevel(level, "default");
                                  else if (v === "__unsupported__") updateThinkingLevel(level, "unsupported");
                                  else updateThinkingLevel(level, v);
                                }}
                                className="rounded-md border border-white/10 bg-zinc-800 px-2 py-1 text-xs text-zinc-200"
                              >
                                <option value="__default__">{t("Default")}</option>
                                <option value="__unsupported__">{t("Unsupported")}</option>
                                {PI_THINKING_LEVELS.filter((l) => l !== "off").map((opt) => (
                                  <option key={opt} value={opt}>
                                    {opt}
                                  </option>
                                ))}
                                {/* allow xhigh as value for max mapping */}
                                {mode !== "default" && mode !== "unsupported" && !PI_THINKING_LEVELS.includes(mode as any) && (
                                  <option value={mode}>{mode}</option>
                                )}
                              </select>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* cost folded */}
            <div className="rounded-lg border border-white/10">
              <button
                type="button"
                onClick={() => setCostExpanded((v) => !v)}
                className="flex w-full items-center justify-between px-3 py-2 text-sm text-zinc-200 hover:bg-white/[0.03]"
              >
                <span className="font-medium">{t("Cost")} <span className="text-xs font-normal text-zinc-500">(input/output/cacheRead/cacheWrite)</span></span>
                <span className="text-xs text-zinc-500">{costExpanded ? "∧" : "∨"}</span>
              </button>
              {costExpanded && (
                <div className="grid gap-2 border-t border-white/10 p-3 sm:grid-cols-2">
                  {[
                    { key: "input", label: "input" },
                    { key: "output", label: "output" },
                    { key: "cacheRead", label: "cacheRead" },
                    { key: "cacheWrite", label: "cacheWrite" },
                  ].map((field) => {
                    const val = (draft.cost as any)?.[field.key] ?? "";
                    return (
                      <div key={field.key}>
                        <label className="mb-1 block text-xs text-zinc-400">{field.label}</label>
                        <Input
                          type="number"
                          step="any"
                          value={String(val ?? "")}
                          onChange={(e) => {
                            const n = e.target.value === "" ? undefined : Number(e.target.value);
                            const nextCost = { ...(draft.cost ?? { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }) } as any;
                            if (n === undefined || Number.isNaN(n)) {
                              delete nextCost[field.key];
                            } else {
                              nextCost[field.key] = n;
                            }
                            update({ cost: nextCost, hasCost: true });
                          }}
                          placeholder="0"
                        />
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            {/* cost warning if missing */}
            {!draft.hasCost && (
              <div className="text-xs text-zinc-500">{t("费用由模型目录自动填充")}</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

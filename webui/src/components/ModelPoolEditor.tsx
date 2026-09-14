import { useEffect, useMemo, useState } from "react";
import type { ModelEntry, ProviderProfile } from "../types";
import { api } from "../api";
import {
  channelNames,
  exposedForChannel,
  materializeMainChannel,
  modelsForChannel,
  parseModelsDraft,
  useProfileDraft,
} from "../hooks/useProfileDraft";
import { draftFromEntry, entryFromDraft, modelPreview, newModelDraft, validateModelsJson, type ModelDraft } from "../lib/piModel";
import { useI18n } from "../i18n";
import { Button, Modal, useAction, useToast } from "./ui";
import { JsonEditor } from "./JsonEditor";
import { ModelCard } from "./ModelCard";

export function ModelPoolEditor({
  name,
  profile,
  onClose,
  onSaved,
}: {
  name: string;
  profile: ProviderProfile;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const run = useAction();
  const toast = useToast();
  const { t, lang } = useI18n() as any;
  // The reducer owns the profile value. ModelCard receives only a derived
  // string-friendly row projection and dispatches changes back to that value.
  const draft = useProfileDraft(materializeMainChannel(profile));
  const [activeChannel, setActiveChannel] = useState<string>("");
  const channels = channelNames(draft.state.value);
  const poolKey = activeChannel || channels[0] || "";
  const rows = draft.state.rows[poolKey] ?? [];
  const drafts = rows.map((row) => ({ ...draftFromEntry(row.model, row.key) }));
  const exposed = exposedForChannel(draft.state.value, poolKey);
  const activeChannelParam = poolKey || undefined;
  const setDrafts = (next: ModelDraft[] | ((prev: ModelDraft[]) => ModelDraft[])) => {
    const nextDrafts = typeof next === "function" ? next(drafts) : next;
    draft.dispatch({ type: "setChannelModels", channel: poolKey, models: nextDrafts.map(entryFromDraft) });
  };
  const setExposed = (next: Set<string> | ((prev: Set<string>) => Set<string>)) => {
    const nextExposed = typeof next === "function" ? next(exposed) : next;
    draft.dispatch({ type: "setExposedSet", channel: poolKey, ids: [...nextExposed] });
  };
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [fetching, setFetching] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const [text, setText] = useState<string>(() => {
    try {
      return JSON.stringify(drafts.map((d) => modelPreview(d)), null, 2);
    } catch {
      return "[]";
    }
  });
  // 切换渠道页签时用该池重播 raw 文本（未应用的 raw 编辑不跨页签保留）。
  useEffect(() => {
    try {
      setText(JSON.stringify(drafts.map((d) => modelPreview(d)), null, 2));
    } catch {
      setText("[]");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [poolKey]);
  useEffect(() => {
    if (mode !== "structured") return;
    try {
      setText(JSON.stringify(drafts.map((d) => modelPreview(d)), null, 2));
    } catch {
      setText("[]");
    }
  }, [draft.state.value, drafts, mode]);
  const jsonValidation = useMemo(() => validateModelsJson(text), [text]);
  const modelsErrorLine = useMemo(() => {
    try {
      JSON.parse(text);
      return null;
    } catch (e) {
      const m = String(e).match(/at position (\d+)/);
      if (m) {
        const pos = Number(m[1]);
        return text.slice(0, pos).split("\n").length;
      }
      const m2 = String(e).match(/line (\d+)/i);
      if (m2) return Number(m2[1]);
      return 1;
    }
  }, [text]);

  function toggleExposed(id: string) {
    setExposed((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function updateDraft(key: string, next: ModelDraft) {
    draft.dispatch({ type: "setModel", channel: poolKey, key, model: entryFromDraft(next) });
  }

  function switchToRaw() {
    try {
      setText(JSON.stringify(drafts.map((d) => modelPreview(d)), null, 2));
    } catch {
      setText("[]");
    }
    setMode("raw");
  }

  function switchToStructured() {
    if (!jsonValidation.ok || !jsonValidation.value) return;
    const nextModels = jsonValidation.value as unknown as ModelEntry[];
    draft.dispatch({ type: "setChannelModels", channel: poolKey, models: nextModels });
    const validIds = new Set(nextModels.map((model) => model.id));
    draft.dispatch({
      type: "setExposedSet",
      channel: poolKey,
      ids: [...exposed].filter((id) => validIds.has(id)),
    });
    setMode("structured");
  }
  function formatModelsJson() {
    try {
      const formatted = JSON.stringify(JSON.parse(text), null, 2);
      setText(formatted);
      const parsed = parseModelsDraft(formatted);
      if (parsed.ok) draft.dispatch({ type: "setChannelModels", channel: poolKey, models: parsed.models });
    } catch {}
  }
  function updateModelsText(nextText: string) {
    setText(nextText);
    const parsed = parseModelsDraft(nextText);
    if (parsed.ok) {
      draft.dispatch({ type: "setChannelModels", channel: poolKey, models: parsed.models });
      setValidationError(null);
    }
  }
  function addModel() {
    // 如果已存在空 ID 的模型，聚焦该行而非新增，避免连续点击产生大量空行
    const empty = drafts.find((d) => !d.id.trim());
    if (empty) {
      setExpandedKeys((s) => {
        const ns = new Set(s);
        ns.add(empty.key);
        return ns;
      });
      // 滚动到该行
      setTimeout(() => {
        document.getElementById(`model-id-${empty.key}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
        document.getElementById(`model-id-${empty.key}`)?.focus();
      }, 50);
      return;
    }
    const d = newModelDraft();
    draft.dispatch({ type: "appendModel", channel: poolKey, key: d.key, model: entryFromDraft(d) });
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.add(d.key);
      return ns;
    });
    setTimeout(() => {
      document.getElementById(`model-id-${d.key}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
      document.getElementById(`model-id-${d.key}`)?.focus();
    }, 50);
  }

  function removeDraft(key: string) {
    draft.dispatch({ type: "removeModel", channel: poolKey, key });
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.delete(key);
      return ns;
    });
  }

  function toggleExpanded(key: string) {
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.has(key) ? ns.delete(key) : ns.add(key);
      return ns;
    });
  }

  async function fetchFromProvider() {
    setFetching(true);
    try {
      const { models: ids, enrich } = await api.fetchModels(name, activeChannelParam);
      setDrafts((prev) => {
        const have = new Set(prev.map((d) => d.id));
        const added = ids.filter((id) => !have.has(id)).map((id) => {
          const d = newModelDraft();
          d.id = id;
          d.name = id;
          d.hasName = true;
          return d;
        });
        return [...prev, ...added];
      });
      // 新拉取的模型默认不暴露：保持暴露集不变，需用户显式勾选暴露
      //（与后端“空 exposed = 不暴露”一致）。
      if (enrich) {
        const isZh = (lang as string) === "zh";
        const base = t("Fetch from provider");
        const enrichMsg =
          enrich.failed > 0
            ? isZh
              ? `上游模型列表 ${ids.length} 条 · 模型元数据 enrich 失败 ${enrich.failed} 条，跳过 ${enrich.skipped} 条，已 enrich ${enrich.enriched} 条`
              : `upstream ${ids.length} · model metadata enrich failed ${enrich.failed}, skipped ${enrich.skipped}, enriched ${enrich.enriched}`
            : isZh
              ? `上游模型列表 ${ids.length} 条 · 已 enrich ${enrich.enriched} 条模型元数据，跳过 ${enrich.skipped} 条（模型目录未覆盖）`
              : `upstream ${ids.length} · enriched ${enrich.enriched} model metadata, skipped ${enrich.skipped} (not in catalog)`;
        const warningPart = enrich.warning ? ` · ${enrich.warning}` : "";
        toast("ok", `${base}: ${enrichMsg}${warningPart}`);
      }
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    } finally {
      setFetching(false);
    }
  }

  function validate(): string | null {
    const seen = new Set<string>();
    for (let i = 0; i < drafts.length; i++) {
      const d = drafts[i];
      if (!d.id.trim()) return `模型 ${i + 1}: 模型 ID 不能为空`;
      if (seen.has(d.id)) return `模型 ID 重复: ${d.id}`;
      seen.add(d.id);
      if (d.hasName && !d.name.trim()) return `模型 ${d.id}: 显示名称不能为空`;
      const cw = d.contextWindow.trim();
      if (cw && (!Number.isFinite(Number(cw)) || Number(cw) <= 0)) return `模型 ${d.id}: 上下文长度必须为正数`;
      const mt = d.maxTokens.trim();
      if (mt && (!Number.isFinite(Number(mt)) || Number(mt) <= 0)) return `模型 ${d.id}: 最大输出 Token 必须为正数`;
    }
    return null;
  }

  const previewJson = (() => {
    try {
      const modelsPreview = drafts.map((d) => modelPreview(d));
      return JSON.stringify({ models: modelsPreview, exposedModels: [...exposed] }, null, 2);
    } catch {
      return "{}";
    }
  })();

  async function saveLocal() {
    let models: ModelEntry[];
    if (mode === "raw") {
      if (!jsonValidation.ok || !jsonValidation.value) {
        const msg = jsonValidation.error ?? "Invalid JSON";
        setValidationError(msg);
        toast("err", msg);
        return;
      }
      models = jsonValidation.value as unknown as ModelEntry[];
      draft.dispatch({ type: "setChannelModels", channel: poolKey, models });
      setValidationError(null);
    } else {
      const err = validate();
      if (err) {
        setValidationError(err);
        toast("err", err);
        const idx = drafts.findIndex((d) => !d.id.trim() || drafts.filter((x) => x.id === d.id).length > 1);
        if (idx >= 0) {
          const key = drafts[idx]?.key;
          if (key) setTimeout(() => document.getElementById(`model-id-${key}`)?.scrollIntoView({ behavior: "smooth", block: "center" }), 50);
        }
        return;
      }
      setValidationError(null);
      models = modelsForChannel(draft.state.value, poolKey);
    }
    const res = await api.updateModels(name, models, activeChannelParam);
    if (res.enrich) {
      const e = res.enrich;
      const isZh = (lang as string) === "zh";
      const enrichMsg =
        e.failed > 0
          ? isZh
            ? `模型元数据 enrich 失败 ${e.failed} 条` + (e.warning ? ` · ${e.warning}` : "")
            : `model metadata enrich failed ${e.failed}` + (e.warning ? ` · ${e.warning}` : "")
          : isZh
            ? `已 enrich ${e.enriched} 条，跳过 ${e.skipped} 条（模型目录未覆盖）` + (e.warning ? ` · ${e.warning}` : "")
            : `enriched ${e.enriched}, skipped ${e.skipped} (not in catalog)` + (e.warning ? ` · ${e.warning}` : "");
      toast("ok", enrichMsg);
    }
    await api.expose(
      name,
      [...exposed].filter((id) => models.some((m) => m.id === id)),
      activeChannelParam,
    );
    toast("ok", "已保存到本地，需到网关发布");
    await onSaved();
  }

  return (
    <>
      <Modal title={`${t("Models")} · ${name}`} onClose={onClose} wide>
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div className="text-sm font-medium text-zinc-200">{t("Model config") || "模型配置"}</div>
          <div className="flex gap-2">
            {mode === "structured" && (
              <>
                <Button onClick={() => void fetchFromProvider()} disabled={fetching} className="h-8">
                  {fetching ? t("Fetching…") : `↓ ${t("Fetch from provider")}`}
                </Button>
                <Button variant="primary" onClick={addModel} className="h-8">
                  + {t("Add model") || "添加模型"}
                </Button>
              </>
            )}
          </div>
        </div>
        {channels.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-1" role="tablist" aria-label="渠道">
            {channels.map((c) => {
              const n = modelsForChannel(draft.state.value, c).length;
              const active = poolKey === c;
              return (
                <button
                  key={c}
                  role="tab"
                  aria-label={c}
                  aria-selected={active}
                  onClick={() => { setActiveChannel(c); }}
                  className={`h-7 rounded-md px-2 text-xs ${active ? "bg-amber-400/20 text-amber-200" : "text-zinc-400 hover:bg-white/5"}`}
                >
                  {c} · {n}
                </button>
              );
            })}
          </div>
        )}
        {mode === "structured" ? (
          <>
            <div className="mb-1 flex items-center gap-2 text-xs text-zinc-500">
              <span className="w-9" />
              <span className="flex-1">{t("Model ID") || "模型 ID"} *</span>
              <span className="flex-1">{t("Display name") || "显示名称"} *</span>
              <span className="w-8" />
            </div>
            <div className="max-h-[42vh] space-y-2 overflow-y-auto rounded-lg border border-white/10 p-2">
              {drafts.length === 0 && (
                <div className="p-3 text-sm text-zinc-500">
                  {t("No models. Add ids above or fetch from the provider.")}
                </div>
              )}
              {drafts.map((d) => (
                <ModelCard
                  key={d.key}
                  draft={d}
                  exposed={exposed.has(d.id)}
                  onToggleExposed={() => toggleExposed(d.id)}
                  onChange={(next) => updateDraft(d.key, next)}
                  onRemove={() => removeDraft(d.key)}
                  expanded={expandedKeys.has(d.key)}
                  onToggleExpanded={() => toggleExpanded(d.key)}
                />
              ))}
            </div>
            <div className="mt-2 text-xs text-zinc-500">
              {t("Configure available models and display names") || "配置可用的模型及其显示名称"} ·{" "}
              <span className="text-zinc-400">{t("Checked = exposed to pi as")}</span> <code>{name}/&lt;channel&gt;/&lt;id&gt;</code>
            </div>
            {validationError && (
              <div className="mt-2 rounded border border-red-500/30 bg-red-500/10 px-2 py-1 text-xs text-red-200">
                {validationError}
              </div>
            )}
            <div className="mt-4">
              <div className="mb-1 text-sm font-medium text-zinc-200">{t("Config JSON") || "配置 JSON"}</div>
              <pre className="max-h-40 overflow-auto rounded-lg border border-white/10 bg-zinc-950 p-2 font-mono text-xs text-zinc-300">
                {previewJson}
              </pre>
              <div className="mt-1 flex justify-end">
                <Button
                  type="button"
                  onClick={() => {
                    navigator.clipboard?.writeText(previewJson).catch(() => {});
                    toast("ok", t("Copied") || "Copied");
                  }}
                  className="h-7 text-xs"
                >
                  {t("Copy") || "复制"}
                </Button>
              </div>
            </div>
          </>
        ) : (
          <div className="space-y-2">
            <JsonEditor value={text} onChange={updateModelsText} label="models json" className="h-64 sm:h-80" errorLine={modelsErrorLine} />
            {!jsonValidation.ok && (
              <div className="rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">Invalid JSON: {jsonValidation.error}</div>
            )}
            {jsonValidation.ok && <div className="text-xs text-emerald-400">✓ JSON valid</div>}
          </div>
        )}
        <div className="mt-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Button onClick={formatModelsJson} className="h-7 text-xs">格式化</Button>
            {mode === "structured" ? (
              <Button onClick={() => switchToRaw()} className="h-7 text-xs">JSON</Button>
            ) : (
              <Button onClick={() => switchToStructured()} className="h-7 text-xs">结构化</Button>
            )}
            <span className="mx-1 text-zinc-600">|</span>
            <button className="text-zinc-400 hover:text-zinc-200 text-xs" onClick={() => setExposed(new Set(drafts.map((d) => d.id)))}>全部暴露</button>
            <button className="text-zinc-400 hover:text-zinc-200 text-xs" onClick={() => setExposed(new Set())}>全部不暴露</button>
          </div>
          <div className="flex gap-2">
            <Button onClick={onClose}>{t("Cancel")}</Button>
            <Button variant="primary" onClick={() => run(() => saveLocal(), undefined)} disabled={mode === "raw" && !jsonValidation.ok}>
              {t("Save")}
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}

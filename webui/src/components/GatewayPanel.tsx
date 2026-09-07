import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { Button, Card, Field, Input, Select, SectionTitle } from "./ui";
import { useI18n } from "../i18n";
import { useAction, useToast } from "./ui";
import { ModelCard } from "./ModelCard";
import { mutateAfterGatewayPublish } from "../store/swr";
import { draftFromEntry, modelPreview, newModelDraft, type ModelDraft } from "../lib/piModel";
import { diffGateway, validateGatewayJson } from "../lib/gatewayDiff";
import { resolveGatewayId, shortGatewayId } from "../lib/gatewayId";
import type { ModelEntry, PreviewGroup } from "../types";
import { JsonEditor } from "./JsonEditor";

const API_OPTIONS = [
  { value: "openai-completions", label: "OpenAI Chat Completions" },
  { value: "openai-responses", label: "OpenAI Responses" },
  { value: "anthropic-messages", label: "Anthropic Messages" },
  { value: "google-generative-ai", label: "Google Gemini" },
  { value: "bedrock-converse-stream", label: "Amazon Bedrock" },
];

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

const LAST_PUBLISH_KEY = "pi-switch-gateway-last-publish";

export function GatewayPanel({ refresh }: { refresh: () => Promise<void> }) {
  const { t } = useI18n() as any;
  const toast = useToast();
  const run = useAction();
  const [current, setCurrent] = useState<Record<string, unknown> | null>(null);
  const [proposed, setProposed] = useState<Record<string, unknown> | null>(null);
  const [conflicts, setConflicts] = useState<string[]>([]);
  const [draft, setDraft] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(true);
  const [drafts, setDrafts] = useState<ModelDraft[]>([]);
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [apiType, setApiType] = useState("openai-completions");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [lastPublishAt, setLastPublishAt] = useState<string | null>(() => {
    try { return typeof window !== "undefined" ? window.localStorage?.getItem(LAST_PUBLISH_KEY) ?? null : null; } catch { return null; }
  });
  const [showMismatchBanner, setShowMismatchBanner] = useState(false);
  const [hasCheckedMismatch, setHasCheckedMismatch] = useState(false);
  const [backendPending, setBackendPending] = useState<number | null>(null);
  // 二次勾选：按供应商/渠道分组的发布选择（网关 id 粒度），默认全选。
  const [groups, setGroups] = useState<PreviewGroup[]>([]);
  const [removedIds, setRemovedIds] = useState<string[]>([]);
  const [checked, setChecked] = useState<Set<string>>(new Set());

  const load = async () => {
    setLoading(true);
    try {
      const preview = await api.previewGateway();
      const cur = (preview as any).current as Record<string, unknown> | null;
      const prop = (preview as any).proposed as Record<string, unknown>;
      const conf = (preview as any).conflicts as string[] ?? [];
      const pending = (preview as any).pending_count as number | undefined;
      setCurrent(cur);
      setProposed(prop);
      setConflicts(conf);
      if (typeof pending === "number") setBackendPending(pending);
      else setBackendPending(null);
      const src = cur ?? prop ?? {};
      setDraft(src);
      const rec = asRecord(src);
      setApiType((rec.api as string) || "openai-completions");
      setBaseUrl((rec.baseUrl as string) || "");
      setApiKey((rec.apiKey as string) || "");
      const models = Array.isArray(rec.models) ? (rec.models as unknown[]) : [];
      setDrafts(models.map((m) => draftFromEntry(m as ModelEntry)));
      // 草稿 id 保持网关全限定形态（supplier/channel/model）：仅渲染层做短显示。
      // 曾在此剥掉 channel 段，导致三段式 id 与历史短 id 撞车、发布又把短 id 写回网关。
      // 分组与二次勾选：groups/removed 透出（旧后端缺省为空），默认全选并集。
      // checked 全程使用网关全限定 id，与分组复选框的键一致；后端 removed 的
      // 历史 id 默认排除，否则每次发布都会把待清理条目写回去。
      const propList = asRecord(prop ?? {}).models;
      const propArr = Array.isArray(propList) ? propList : [];
      const propIds = propArr.map((m) => String((m as any)?.id ?? "")).filter(Boolean);
      const draftIds = models.map((m) => String((m as any)?.id ?? "")).filter(Boolean);
      const removedList = (preview as any).removed;
      const removedArr = Array.isArray(removedList) ? removedList.map((id) => String(id)) : [];
      const removedSet = new Set(removedArr);
      setChecked(new Set([...propIds, ...draftIds].filter((id) => !removedSet.has(id))));
      setGroups(Array.isArray((preview as any).groups) ? ((preview as any).groups as PreviewGroup[]) : []);
      setRemovedIds(removedArr);
      skipAutoCheck.current.clear();
      // 后端 removed 的历史 id 同样记入跳过集，否则下面的自动补勾 effect
      // 会立刻把它们加回 checked（清理永远发不出去）。用户在“已撤回”区
      // 显式勾选时 toggleChecked 会将其移出跳过集，仍可复活。
      for (const id of removedSet) skipAutoCheck.current.add(id);
      // 首次进入若 preview diff 非空，顶部提示是否立即同步，默认不自动写
      if (!hasCheckedMismatch) {
        const curForDiff = cur as Record<string, unknown> | null;
        const propForDiff = prop as Record<string, unknown>;
        if (propForDiff) {
          const pendingVal = typeof pending === "number" ? pending : null;
          const hasDiff = pendingVal !== null ? pendingVal > 0 : (() => {
            const d = diffGateway(curForDiff, propForDiff);
            return d.added.length > 0 || d.removed.length > 0 || d.changed.length > 0;
          })();
          if (hasDiff) setShowMismatchBanner(true);
        }
        setHasCheckedMismatch(true);
      }
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const liveJson = useMemo(() => {
    if (!draft) return "{}";
    const modelsPreview = drafts.map((d) => modelPreview(d));
    const next: Record<string, unknown> = {
      ...draft,
      api: apiType,
      baseUrl: baseUrl.trim(),
      ...(apiKey ? { apiKey } : {}),
      models: modelsPreview,
    };
    if (!apiKey) delete (next as any).apiKey;
    return JSON.stringify(next, null, 2);
  }, [draft, drafts, apiType, baseUrl, apiKey]);

  const validation = useMemo(() => validateGatewayJson(liveJson), [liveJson]);
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const [rawText, setRawText] = useState(liveJson);
  const rawValidation = useMemo(() => validateGatewayJson(rawText), [rawText]);
  const gatewayErrorLine = useMemo(() => {
    try {
      JSON.parse(rawText);
      return null;
    } catch (e) {
      const m = String(e).match(/at position (\d+)/);
      if (m) {
        const pos = Number(m[1]);
        return rawText.slice(0, pos).split("\n").length;
      }
      const m2 = String(e).match(/line (\d+)/i);
      if (m2) return Number(m2[1]);
      return 1;
    }
  }, [rawText]);
  useEffect(() => {
    setRawText(liveJson);
  }, [liveJson]);
  function switchToStructuredFromRaw() {
    if (rawValidation.ok && rawValidation.value) {
      const rec = asRecord(rawValidation.value);
      setApiType((rec.api as string) || "openai-completions");
      setBaseUrl((rec.baseUrl as string) || "");
      setApiKey((rec.apiKey as string) || "");
      const models = Array.isArray(rec.models) ? (rec.models as unknown[]) : [];
      setDrafts(models.map((m) => draftFromEntry(m as ModelEntry)));
    }
    setMode("structured");
  }
  function formatRaw() {
    try {
      setRawText(JSON.stringify(JSON.parse(rawText), null, 2));
    } catch {}
  }

  // diff for status bar: Current vs Proposed (backend) – pending publish count
  const statusDiff = useMemo(() => {
    if (!proposed) return { added: [], removed: [], changed: [] };
    return diffGateway(current, proposed as Record<string, unknown>);
  }, [current, proposed]);

  const pendingCount = backendPending ?? (statusDiff.added.length + statusDiff.removed.length + statusDiff.changed.length);

  // preview diff for mismatch banner (current vs proposed before edits)
  const previewDiff = useMemo(() => {
    if (!proposed) return null;
    return diffGateway(current, proposed);
  }, [current, proposed]);

  function addModel() {
    const empty = drafts.find((d) => !d.id.trim());
    if (empty) {
      setExpandedKeys((s) => {
        const ns = new Set(s);
        ns.add(empty.key);
        return ns;
      });
      setTimeout(() => {
        document.getElementById(`model-id-${empty.key}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
        document.getElementById(`model-id-${empty.key}`)?.focus();
      }, 50);
      return;
    }
    const d = newModelDraft();
    setDrafts((prev) => [...prev, d]);
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

  // 新出现的模型 id 默认纳入发布选择；用户显式取消的不再补回。
  const skipAutoCheck = useRef<Set<string>>(new Set());
  useEffect(() => {
    const ids = new Set<string>();
    for (const d of drafts) if (d.id.trim()) ids.add(d.id);
    const propModels = asRecord(proposed ?? {}).models;
    if (Array.isArray(propModels)) {
      for (const m of propModels as Array<unknown>) ids.add(String((m as any)?.id ?? ""));
    }
    const rawModels = rawValidation.value?.models;
    if (Array.isArray(rawModels)) {
      for (const m of rawModels as Array<unknown>) ids.add(String((m as any)?.id ?? ""));
    }
    ids.delete("");
    setChecked((prev) => {
      let changed = false;
      const next = new Set(prev);
      for (const id of ids) if (!next.has(id) && !skipAutoCheck.current.has(id)) { next.add(id); changed = true; }
      return changed ? next : prev;
    });
  }, [drafts, proposed, rawValidation.value]);


  // 输入框短显示 ↔ 全限定数据的映射基准：提议与已注入的全部全量 id。
  const knownGatewayIds = useMemo(() => {
    const ids = new Set<string>();
    for (const src of [proposed, current]) {
      const models = asRecord(src ?? {}).models;
      if (Array.isArray(models)) {
        for (const m of models) {
          const id = String((m as any)?.id ?? "");
          if (id) ids.add(id);
        }
      }
    }
    return ids;
  }, [proposed, current]);

  // 草稿行 id 写回：输入框给的是短显示文本，映射回全限定 id 再落草稿。
  function handleDraftChange(prev: ModelDraft, next: ModelDraft) {
    if (next.id !== prev.id) {
      next = { ...next, id: resolveGatewayId(next.id, prev.id, knownGatewayIds) };
    }
    const mapped = next;
    setDrafts((drafts) => drafts.map((x) => (x.key === prev.key ? mapped : x)));
  }
  function gatewayIdOf(g: PreviewGroup, itemId: string): string {
    return g.channel ? `${g.supplier}/${g.channel}/${itemId}` : `${g.supplier}/${itemId}`;
  }
  function displayGatewayId(g: PreviewGroup, itemId: string): string {
    // 前端展示隐藏 channel 段，仅显示 supplier/model
    return `${g.supplier}/${itemId}`;
  }

  // 二次勾选子集：按勾选过滤发布模型；有草稿行优先用用户编辑，无行用候选/已注入原文。
  function buildSelectedModels(): Array<Record<string, unknown>> {
    const draftById = new Map(drafts.map((d) => [d.id, modelPreview(d) as unknown as Record<string, unknown>]));
    const propById = new Map<string, Record<string, unknown>>();
    const propModels = asRecord(proposed ?? {}).models;
    if (Array.isArray(propModels)) {
      for (const m of propModels as Array<Record<string, unknown>>) {
        const id = String((m as any)?.id ?? "");
        if (id) propById.set(id, m);
      }
    }
    const curById = new Map<string, Record<string, unknown>>();
    const curModels = asRecord(current ?? {}).models;
    if (Array.isArray(curModels)) {
      for (const m of curModels as Array<Record<string, unknown>>) {
        const id = String((m as any)?.id ?? "");
        if (id) curById.set(id, m);
      }
    }
    const order: string[] = [];
    for (const id of propById.keys()) order.push(id);
    for (const id of draftById.keys()) if (!order.includes(id)) order.push(id);
    for (const id of curById.keys()) if (!order.includes(id)) order.push(id);
    const out: Array<Record<string, unknown>> = [];
    for (const id of order) {
      if (!checked.has(id)) continue;
      out.push(draftById.get(id) ?? propById.get(id) ?? curById.get(id) ?? { id });
    }
    return out;
  }

  // 勾选子集待发布数：当前已注入 vs 勾选子集的按模型差异计数（本地计算，不调后端）。
  const subsetPending = useMemo(() => {
    const selected = buildSelectedModels();
    const curModels = asRecord(current ?? {}).models;
    const curById = new Map<string, string>();
    if (Array.isArray(curModels)) {
      for (const m of curModels as Array<Record<string, unknown>>) {
        const id = String((m as any)?.id ?? "");
        if (id) curById.set(id, JSON.stringify(m));
      }
    }
    let n = 0;
    const seen = new Set<string>();
    for (const m of selected) {
      const id = String((m as any)?.id ?? "");
      seen.add(id);
      if (curById.get(id) !== JSON.stringify(m)) n++;
    }
    for (const id of curById.keys()) if (!seen.has(id)) n++;
    return n;
  }, [current, drafts, proposed, checked]);

  function toggleChecked(id: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); skipAutoCheck.current.add(id); }
      else { next.add(id); skipAutoCheck.current.delete(id); }
      return next;
    });
  }

  async function handleApplyToPi() {
    const activeValidation = rawValidation;
    if (!activeValidation.ok || !activeValidation.value) {
      toast("err", activeValidation.error ?? "Invalid JSON");
      return;
    }
    const payload = activeValidation.value as Record<string, unknown>;
    const newApi = typeof payload.api === "string" ? payload.api : "";
    const newBaseUrl = typeof payload.baseUrl === "string" ? payload.baseUrl : "";
    try {
      await api.applyGateway({ ...activeValidation.value, models: buildSelectedModels() });
      // 同步网关的 api / baseUrl 回 Settings（最小实现：api 直写 gatewayApi，baseUrl 解析 host:port）
      try {
        const state = await api.getState();
        const curApi = state.settings.gatewayApi;
        const curHost = state.settings.proxy.host;
        const curPort = state.settings.proxy.port;
        let needUpdate = false;
        const nextSettings = JSON.parse(JSON.stringify(state.settings)) as typeof state.settings;
        if (newApi && newApi !== curApi) {
          (nextSettings as any).gatewayApi = newApi;
          needUpdate = true;
        }
        if (newBaseUrl) {
          try {
            const u = new URL(newBaseUrl);
            const newHost = u.hostname;
            const rawPort = u.port;
            // 仅当解析出的 host/port 与当前不一致时同步；0.0.0.0 归一化为 127.0.0.1 与后端一致
            const normalizedNewHost = newHost === "0.0.0.0" || newHost === "::" || newHost === "[::]" ? "127.0.0.1" : newHost;
            const normalizedCurHost = curHost === "0.0.0.0" || curHost === "::" || curHost === "[::]" ? "127.0.0.1" : curHost;
            if (normalizedNewHost && normalizedNewHost !== normalizedCurHost) {
              nextSettings.proxy.host = normalizedNewHost;
              needUpdate = true;
            }
            if (rawPort) {
              const newPort = parseInt(rawPort, 10);
              if (newPort && newPort !== curPort) {
                nextSettings.proxy.port = newPort;
                needUpdate = true;
              }
            }
          } catch {}
        }
        if (needUpdate) {
          await api.updateSettings(nextSettings);
        }
      } catch (e) {
        // Settings 同步失败不阻断已成功的 gateway 写入，仅提示
        console.warn("[gateway] sync settings failed", e);
      }
      const now = new Date().toISOString();
      try { if (typeof window !== "undefined") window.localStorage?.setItem(LAST_PUBLISH_KEY, now); } catch {}
      setLastPublishAt(now);
      setShowMismatchBanner(false);
      toast("ok", t("Saved") || "Saved");
      await mutateAfterGatewayPublish();
      await load();
      await refresh();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
      // 失败保留 config，不 reload
    }
  }

  if (loading) return <div className="text-sm text-zinc-500">{t("Loading…")}</div>;

  const tOr = (k: string, fallback: string) => (t(k) !== k ? t(k) : fallback);

  const lastPublishLabel = (() => {
    if (lastPublishAt) {
      try { return new Date(lastPublishAt).toLocaleString(); } catch { return lastPublishAt; }
    }
    if (!current) return "尚未发布";
    return "未知";
  })();

  return (
    <div>
      <SectionTitle hint={t("Provider config injected into ./pi/agent/models.json") || "Gateway injection"}>
        {t("Gateway") || "Gateway"}
      </SectionTitle>

      {/* Current vs Proposed 状态条 */}
      <div className="mb-3 rounded-lg border border-white/10 bg-zinc-900/50 px-3 py-2">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="font-medium text-zinc-200">Current vs Proposed</span>
          <span className="text-zinc-500">·</span>
          <span className="text-emerald-300">+{statusDiff.added.length} added</span>
          <span className="text-red-300">-{statusDiff.removed.length} removed</span>
          <span className="text-zinc-300">~{statusDiff.changed.length} changed</span>
          <span className="text-zinc-500">·</span>
          <span className="text-zinc-200">待发布数: {pendingCount}</span>
          <span className="text-zinc-500">·</span>
          <span className="text-zinc-400">上次发布时间: {lastPublishLabel}</span>
        </div>
        {conflicts.length > 0 && (
          <div className="mt-1 text-xs text-amber-300">冲突: {conflicts.join(", ")}</div>
        )}
      </div>

      {/* 首次进入不一致提示，默认不自动写 */}
      {showMismatchBanner && previewDiff && (previewDiff.added.length + previewDiff.removed.length + previewDiff.changed.length > 0) && (
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2">
          <span className="text-sm text-amber-200">检测到本地与 Pi 网关不一致，是否立即同步</span>
          <div className="flex gap-2">
            <Button variant="primary" onClick={() => void run(() => handleApplyToPi(), undefined)} className="h-7 text-xs">
              立即同步
            </Button>
            <Button onClick={() => setShowMismatchBanner(false)} className="h-7 text-xs">
              稍后
            </Button>
          </div>
        </div>
      )}

      {/* 已暴露候选分组 + 二次勾选：只读归属，暴露编辑仍在供应商页 */}
      {groups.length > 0 && (
        <div className="mb-3 rounded-lg border border-white/10 bg-zinc-900/50 px-3 py-2">
          <div className="mb-2 flex items-center justify-between text-xs font-medium text-zinc-200">
            <span>候选分组（按供应商/渠道，勾选后发布）</span>
            <span className="text-zinc-400">勾选子集待发布：{subsetPending}</span>
          </div>
          <div className="space-y-2">
            {groups.map((g) => {
              const title = g.channel ? `${g.supplier} / ${g.channel}` : g.supplier;
              const pub = g.models.filter((m) => m.status === "published").length;
              return (
                <div key={title} className="rounded border border-white/10 px-2 py-1">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-medium text-zinc-200">{title}</span>
                    <span className="text-zinc-500">已发布 {pub} / 待发布 {g.models.length - pub}</span>
                  </div>
                  {g.models.length === 0 && (
                    <div className="mt-1 text-xs text-zinc-500">该渠道未暴露模型，去供应商页勾选后发布</div>
                  )}
                  {g.models.map((m) => {
                    const gid = gatewayIdOf(g, m.id);
                    return (
                      <label key={gid} className="mt-1 flex items-center gap-2 text-xs text-zinc-300">
                        <input
                          type="checkbox"
                          aria-label={gid}
                          checked={checked.has(gid)}
                          onChange={() => toggleChecked(gid)}
                          className="h-3.5 w-3.5 accent-amber-400"
                        />
                        <span className="font-mono">{displayGatewayId(g, m.id)}</span>
                        <span className={m.status === "published" ? "text-emerald-400" : "text-amber-300"}>
                          {m.status === "published" ? "已发布" : "待发布"}
                        </span>
                      </label>
                    );
                  })}
                </div>
              );
            })}
            {removedIds.length > 0 && (
              <div className="rounded border border-white/10 px-2 py-1">
                <div className="text-xs font-medium text-zinc-400">已撤回待同步</div>
                {removedIds.map((id) => (
                  <label key={id} className="mt-1 flex items-center gap-2 text-xs text-zinc-400">
                    <input
                      type="checkbox"
                      aria-label={id}
                      checked={checked.has(id)}
                      onChange={() => toggleChecked(id)}
                      className="h-3.5 w-3.5 accent-amber-400"
                    />
                    <span className="font-mono">{(() => { const p = id.split("/"); return p.length === 3 ? `${p[0]}/${p[2]}` : id; })()}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
      <Card className="mb-4">
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label={t("接口格式")}>
            <Select value={apiType} onChange={(e) => setApiType(e.target.value)}>
              {API_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
              {!API_OPTIONS.some((o) => o.value === apiType) && apiType && (
                <option value={apiType}>{apiType}</option>
              )}
            </Select>
            <p className="mt-1 text-xs text-zinc-500">{t("修改后应用到 Pi 时将同步到 Settings → Gateway API") || "修改后应用到 Pi 时将同步到 Settings → Gateway API"}</p>
          </Field>
          <Field label={t("Base URL")}>
            <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.example.com/v1" />
            <p className="mt-1 text-xs text-zinc-500">{t("修改后应用到 Pi 时将同步到 Settings → Proxy host:port，0.0.0.0 已归一化为 127.0.0.1") || "修改后应用到 Pi 时将同步到 Settings → Proxy host:port（0.0.0.0 已归一化为 127.0.0.1）"}</p>
          </Field>
          <div className="sm:col-span-2">
            <Field label={t("API key")}>
              <Input value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-…" />
            </Field>
          </div>
        </div>


        {/* Models section — cc-switch style */}
        <div className="mt-6 border-l border-white/10 pl-3">
          <div className="flex items-center justify-between gap-3">
            <div className="text-sm font-medium text-zinc-200">{t("Model config")}</div>
            <div className="flex gap-2">
              <Button type="button" variant="primary" onClick={addModel} className="h-8">
                + {t("Add model")}
              </Button>
            </div>
          </div>
          <div className="mt-1 text-xs text-zinc-500">
            {t("Configure available models and display names")}
          </div>

          <div className="mt-3 space-y-2">
            <div className="hidden sm:grid grid-cols-[auto_1fr_1fr_auto] gap-2 px-1 text-xs text-zinc-500">
              <span className="w-9" />
              <span>{t("Model ID")} *</span>
              <span>{t("Display name")} *</span>
              <span className="w-8" />
            </div>
            {drafts.length === 0 && (
              <div className="rounded-lg border border-dashed border-white/10 p-4 text-center text-sm text-zinc-500">
                {t("No models configured")}
              </div>
            )}
            {drafts.map((d) => (
              <ModelCard
                key={d.key}
                draft={d}
                exposed={true}
                hideExposed
                onToggleExposed={() => {}}
                onChange={(next) => handleDraftChange(d, next)}
                displayId={shortGatewayId(d.id)}
                fullId={d.id}
                onRemove={() => {
                  // 删行等价于取消勾选：否则 buildSelectedModels 会从
                  // proposed/current 按 id 取回该行，删了也发回去（删不掉）。
                  setChecked((prev) => {
                    const ns = new Set(prev);
                    ns.delete(d.id);
                    return ns;
                  });
                  skipAutoCheck.current.add(d.id);
                  setDrafts((prev) => prev.filter((x) => x.key !== d.key));
                }}
                expanded={expandedKeys.has(d.key)}
                onToggleExpanded={() =>
                  setExpandedKeys((s) => {
                    const ns = new Set(s);
                    ns.has(d.key) ? ns.delete(d.key) : ns.add(d.key);
                    return ns;
                  })
                }
              />
            ))}
          </div>
        </div>
      </Card>
        {/* Live JSON preview — editable */}
        <div className="mt-6">
          <div className="mb-1 text-sm font-medium text-zinc-200">{t("Config JSON")}</div>
          <div className="space-y-2">
            <JsonEditor value={rawText} onChange={setRawText} label="gateway json" className="h-80" errorLine={gatewayErrorLine} />
            {!rawValidation.ok && (
              <div className="rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">Invalid JSON: {rawValidation.error}</div>
            )}
            {rawValidation.ok && <div className="text-xs text-emerald-400">✓ JSON valid</div>}
          </div>
          <div className="mt-3 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Button onClick={formatRaw} className="h-7 text-xs">格式化</Button>
            </div>
            <div className="flex gap-2">
              <Button onClick={() => void load()}>{t("Cancel")}</Button>
              <Button
                variant="primary"
                disabled={!rawValidation.ok}
                onClick={() => void run(() => handleApplyToPi(), t("Saved") || "Saved")}
              >
                应用到 Pi
              </Button>
            </div>
          </div>
        </div>
    </div>
  );
}

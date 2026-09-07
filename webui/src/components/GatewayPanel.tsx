import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { Button, Card, SectionTitle } from "./ui";
import { useI18n } from "../i18n";
import { useAction, useToast } from "./ui";
import { ModelCard } from "./ModelCard";
import { mutateAfterGatewayPublish } from "../store/swr";
import { draftFromEntry, modelPreview, newModelDraft, type ModelDraft } from "../lib/piModel";
import { diffGateway, validateGatewayJson } from "../lib/gatewayDiff";
import { addUncheckedId, loadUncheckedIds, removeUncheckedId } from "../lib/gatewayUnchecked";
import type { ModelEntry, PreviewGroup } from "../types";
import { JsonEditor } from "./JsonEditor";

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

function hasLegacyGatewayModelIds(value: Record<string, unknown> | null): boolean {
  return Object.values(value ?? {}).some((entry) => {
    const models = asRecord(entry).models;
    return Array.isArray(models) && models.some((model) => String(asRecord(model).id ?? "").includes("/"));
  });
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
  const [lastPublishAt, setLastPublishAt] = useState<string | null>(() => {
    try { return typeof window !== "undefined" ? window.localStorage?.getItem(LAST_PUBLISH_KEY) ?? null : null; } catch { return null; }
  });
  const [backendPending, setBackendPending] = useState<number | null>(null);
  // 二次勾选：按供应商/渠道分组的发布选择（网关 id 粒度），默认只勾选已发布。
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
      const src = (cur && Object.keys(cur).length > 0 && !hasLegacyGatewayModelIds(cur) ? cur : prop ?? {}) as Record<string, unknown>;
      setDraft(src);
      let draftModels: unknown[] = [];
      let propIds: string[] = [];
      let publishedIds: Set<string>;
      const allModels: unknown[] = [];
      for (const [providerKey, entry] of Object.entries(src)) {
        const models = asRecord(entry).models;
        if (Array.isArray(models)) {
          for (const model of models) allModels.push({ providerKey, entry: model });
        }
      }
      draftModels = allModels;
      setDrafts(allModels.map((m) => {
        const model = m as Record<string, unknown>;
        return { ...draftFromEntry(model.entry as ModelEntry), providerKey: String(model.providerKey ?? "") };
      }));
      const providerModelIds = (value: Record<string, unknown> | null) => Object.entries(value ?? {}).flatMap(([pk, entry]) => {
        const models = asRecord(entry).models;
        return Array.isArray(models) ? models.map((m) => `${pk}/${String((m as Record<string, unknown>).id ?? "")}`).filter((id) => !id.endsWith("/")) : [];
      });
      propIds = providerModelIds(prop);
      publishedIds = new Set(providerModelIds(cur));
      const draftIds = draftModels.map((m) => {
        const model = m as Record<string, unknown>;
        return `${String(model.providerKey ?? "")}/${String(asRecord(model.entry).id ?? "")}`;
      }).filter((id) => !id.endsWith("/"));
      const removedList = (preview as any).removed;
      const removedArr = Array.isArray(removedList) ? removedList.map((id) => String(id)) : [];
      const removedSet = new Set(removedArr);
      // 用户的历史取消勾选：跨 load 持久。
      const knownIds = new Set([...propIds, ...draftIds]);
      const persistedUnchecked = loadUncheckedIds(knownIds);
      setChecked(
        new Set(
          [...propIds, ...draftIds].filter(
            (id) => publishedIds.has(id) && !removedSet.has(id) && !persistedUnchecked.has(id),
          ),
        ),
      );
      setGroups(Array.isArray((preview as any).groups) ? ((preview as any).groups as PreviewGroup[]) : []);
      setRemovedIds(removedArr);
      skipAutoCheck.current.clear();
      // 后端 removed 的历史 id 同样记入跳过集，否则下面的自动补勾 effect
      // 会立刻把它们加回 checked（清理永远发不出去）。用户在“已撤回”区
      // 显式勾选时 toggleChecked 会将其移出跳过集，仍可复活。
      for (const id of removedSet) skipAutoCheck.current.add(id);
      // 持久化的用户排除同样记入跳过集，防自动补勾 effect 加回；
      // 显式勾选时 toggleChecked 会同步清除持久记录。
      for (const id of persistedUnchecked) skipAutoCheck.current.add(id);
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
    return JSON.stringify({ providers: draft }, null, 2);
  }, [draft]);

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
      const providers = asRecord(asRecord(rawValidation.value).providers);
      setDraft(providers);
      const models = Object.entries(providers).flatMap(([providerKey, entry]) => {
        const providerModels = asRecord(entry).models;
        return Array.isArray(providerModels) ? providerModels.map((m) => ({ providerKey, entry: m })) : [];
      });
      setDrafts(models.map((m) => {
        const model = m as Record<string, unknown>;
        return { ...draftFromEntry(model.entry as ModelEntry), providerKey: String(model.providerKey ?? "") };
      }));
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
    const providerKey = Object.keys((proposed ?? current ?? {}) as Record<string, unknown>)[0] ?? "";
    const d = { ...newModelDraft(), providerKey };
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

  // 提议/草稿 id 永不自动补勾：没发布的默认不勾选，只认打勾与改 ID。
  const skipAutoCheck = useRef<Set<string>>(new Set());


  function handleDraftChange(prev: ModelDraft, next: ModelDraft) {
    if (next.id !== prev.id) {
      // 裸 id 场景：直接使用输入的裸 id，无需短显映射
      if (next.id.trim()) {
        const providerKey = next.providerKey ?? prev.providerKey ?? Object.keys((proposed ?? current ?? {}) as Record<string, unknown>)[0] ?? "";
        const checkedId = `${providerKey}/${next.id}`;
        skipAutoCheck.current.delete(checkedId);
        removeUncheckedId(checkedId);
        setChecked((prevChecked) => new Set(prevChecked).add(checkedId));
      }
    }
    const mapped = next;
    setDrafts((drafts) => drafts.map((x) => (x.key === prev.key ? mapped : x)));
  }
  function providerModelKey(g: PreviewGroup, itemId: string): string {
    return g.channel ? `${g.supplier}/${g.channel}/${itemId}` : `${g.supplier}/${itemId}`;
  }
  function displayGatewayId(_g: PreviewGroup, itemId: string): string {
    return itemId;
  }

  // 二次勾选子集：按勾选过滤发布模型，构建 providers map
  function buildSelectedProviders(): Record<string, Record<string, unknown>> {
    const result: Record<string, Record<string, unknown>> = {};
    const draftById = new Map(drafts.filter((d) => d.providerKey && d.id).map((d) => [`${d.providerKey}/${d.id}`, modelPreview(d) as unknown as Record<string, unknown>]));
    const propById = new Map<string, Record<string, unknown>>();
    const curById = new Map<string, Record<string, unknown>>();
    for (const src of [proposed, current]) {
      if (!src) continue;
      for (const [providerKey, entry] of Object.entries(src as Record<string, unknown>)) {
        const models = asRecord(entry).models;
        if (Array.isArray(models)) {
          for (const m of models as Array<Record<string, unknown>>) {
            const id = String(m.id ?? "");
            if (!id) continue;
            const gid = `${providerKey}/${id}`;
            if (src === proposed) propById.set(gid, m);
            else curById.set(gid, m);
          }
        }
      }
    }
    for (const [id, m] of draftById) {
      if (!propById.has(id) && !curById.has(id)) {
        propById.set(id, m);
      }
    }
    const order: string[] = [];
    for (const id of propById.keys()) order.push(id);
    for (const id of draftById.keys()) if (!order.includes(id)) order.push(id);
    for (const id of curById.keys()) if (!order.includes(id)) order.push(id);
    for (const gid of order) {
      if (!checked.has(gid)) continue;
      const lastSlash = gid.lastIndexOf("/");
      const providerKey = gid.slice(0, lastSlash);
      const bareId = gid.slice(lastSlash + 1);
      const modelEntry = draftById.get(gid) ?? propById.get(gid) ?? curById.get(gid) ?? { id: bareId };
      const entryWithBare = { ...(modelEntry as Record<string, unknown>), id: bareId };
      if (!result[providerKey]) {
        const srcEntry = (proposed as Record<string, unknown>)?.[providerKey] as Record<string, unknown> | undefined;
        const curEntry = (current as Record<string, unknown>)?.[providerKey] as Record<string, unknown> | undefined;
        const base = (srcEntry as Record<string, unknown>) || (curEntry as Record<string, unknown>) || {};
        result[providerKey] = {
          api: (base["api"] as string) || "openai-completions",
          baseUrl: (base["baseUrl"] as string) || "",
          apiKey: (base["apiKey"] as string) || "pi-switch-proxy",
          models: [],
          proxy: false,
        };
        if (base["compat"] && typeof base["compat"] === "object" && !Array.isArray(base["compat"])) {
          (result[providerKey] as Record<string, unknown>)["compat"] = base["compat"];
        }
        if ((base as Record<string, unknown>)["apiKey"]) {
          (result[providerKey] as Record<string, unknown>)["apiKey"] = (base as Record<string, unknown>)["apiKey"];
        }
      }
      const prov = result[providerKey] as Record<string, unknown>;
      const models = prov["models"] as Array<Record<string, unknown>>;
      models.push(entryWithBare as Record<string, unknown>);
    }
    return result;
  }

  // 勾选子集待发布数：当前已注入 vs 勾选子集的按模型差异计数（本地计算，不调后端）。
  const subsetPending = useMemo(() => {
    const isProvidersMap = (obj: Record<string, unknown> | null) => {
      if (!obj || typeof obj !== "object" || Array.isArray(obj)) return false;
      const vals = Object.values(obj);
      return vals.length > 0 && vals.some((v) => v && typeof v === "object" && !Array.isArray(v) && "models" in (v as Record<string, unknown>));
    };
    if (isProvidersMap(current as Record<string, unknown>)) {
      const curById = new Map<string, string>();
      for (const [pk, entry] of Object.entries(current as Record<string, unknown>)) {
        const rec = entry as Record<string, unknown>;
        const models = rec?.["models"] as unknown[] | undefined;
        if (Array.isArray(models)) {
          for (const m of models as Array<Record<string, unknown>>) {
            const id = String((m as Record<string, unknown>)?.["id"] ?? "");
            if (id) curById.set(`${pk}/${id}`, JSON.stringify({ ...m, id }));
          }
        }
      }
      const selectedProviders = buildSelectedProviders();
      let n = 0;
      const seen = new Set<string>();
      for (const [pk, prov] of Object.entries(selectedProviders)) {
        const models = (prov as Record<string, unknown>)["models"] as Array<Record<string, unknown>> | undefined;
        if (!Array.isArray(models)) continue;
        for (const m of models) {
          const id = String((m as Record<string, unknown>)?.["id"] ?? "");
          const gid = `${pk}/${id}`;
          seen.add(gid);
          const curJson = curById.get(gid);
          const selJson = JSON.stringify({ ...m, id });
          if (curJson !== selJson) n++;
        }
      }
      for (const gid of curById.keys()) if (!seen.has(gid)) n++;
      return n;
    }
    return 0;
  }, [current, drafts, proposed, checked]);

  function toggleChecked(id: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); skipAutoCheck.current.add(id); addUncheckedId(id); }
      else { next.add(id); skipAutoCheck.current.delete(id); removeUncheckedId(id); }
      return next;
    });
  }

  async function handleApplyToPi() {
    const activeValidation = rawValidation;
    if (!activeValidation.ok || !activeValidation.value) {
      toast("err", activeValidation.error ?? "Invalid JSON");
      return;
    }
    const activeVal = activeValidation.value as Record<string, unknown>;
    const isProvidersMapDraft = (() => {
      const obj = proposed as Record<string, unknown> | null;
      if (!obj) return false;
      const vals = Object.values(obj);
      return vals.length > 0 && vals.some((v) => v && typeof v === "object" && !Array.isArray(v) && "models" in (v as Record<string, unknown>));
    })();
    if (!("providers" in activeVal) && !isProvidersMapDraft) {
      toast("err", "Gateway JSON must contain a providers object");
      return;
    }
    const selectedProviders = buildSelectedProviders();
    const payloadToSend: Record<string, unknown> = { providers: selectedProviders };
    const providerValues = Object.values(asRecord(activeVal.providers));
    const newBaseUrl = typeof asRecord(providerValues[0]).baseUrl === "string" ? String(asRecord(providerValues[0]).baseUrl) : "";
    try {
      await api.applyGateway(payloadToSend);
      // 仅同步本地 proxy host:port；每个 gateway provider 的 API 来自 channel.api。
      try {
        const state = await api.getState();
        const curHost = state.settings.proxy.host;
        const curPort = state.settings.proxy.port;
        let needUpdate = false;
        const nextSettings = JSON.parse(JSON.stringify(state.settings)) as typeof state.settings;
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
                    const gid = providerModelKey(g, m.id);
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
                    <span className="font-mono">{id}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
      <Card className="mb-4">

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
                displayId={d.id}
                fullId={d.id}
                onRemove={() => {
                  // 删行等价于取消勾选：否则 buildSelectedProviders 会从
                  // proposed/current 按 id 取回该行，删了也发回去（删不掉）。
                  const gid = d.providerKey ? `${d.providerKey}/${d.id}` : d.id;
                  setChecked((prev) => {
                    const ns = new Set(prev);
                    ns.delete(gid);
                    return ns;
                  });
                  skipAutoCheck.current.add(gid);
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

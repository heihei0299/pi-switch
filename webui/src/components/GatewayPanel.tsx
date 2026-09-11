import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { Button, Card, SectionTitle } from "./ui";
import { useI18n } from "../i18n";
import { useToast } from "./ui";
import { mutateAfterGatewayPublish } from "../store/swr";
import { draftFromEntry, modelPreview, type ModelDraft } from "../lib/piModel";
import {
  filterFixedGatewayDiff,
  filterFixedGatewayProviders,
  isFixedGatewayProvider,
  makeFixedProviderSet,
  validateGatewayJson,
  type FixedProviderSet,
} from "../lib/gatewayDiff";
import { addUncheckedId, loadUncheckedIds, removeUncheckedId } from "../lib/gatewayUnchecked";
import type { GatewayDiff, GatewayPreview, GatewaySelection, ModelEntry, PreviewGroup } from "../types";
import { JsonEditor } from "./JsonEditor";
import { ModelCard } from "./ModelCard";

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {};
}

const LAST_PUBLISH_KEY = "pi-switch-gateway-last-publish";

export function GatewayPanel({ refresh }: { refresh: () => Promise<void> }) {
  const { t } = useI18n() as any;
  const toast = useToast();
  const [current, setCurrent] = useState<Record<string, unknown> | null>(null);
  const [conflicts, setConflicts] = useState<string[]>([]);
  const [backendDiff, setBackendDiff] = useState<GatewayDiff>({ added: [], removed: [], changed: [] });
  const [canonicalDraft, setCanonicalDraft] = useState<Record<string, unknown> | null>(null);
  const [proposedDraft, setProposedDraft] = useState<Record<string, unknown> | null>(null);
  const [fullProposedDraft, setFullProposedDraft] = useState<Record<string, unknown> | null>(null);
  const [draftView, setDraftView] = useState<"current" | "proposed">("current");
  const [loading, setLoading] = useState(true);
  const [lastPublishAt, setLastPublishAt] = useState<string | null>(() => {
    try { return typeof window !== "undefined" ? window.localStorage?.getItem(LAST_PUBLISH_KEY) ?? null : null; } catch { return null; }
  });
  const [backendPending, setBackendPending] = useState<number | null>(null);
  const [rawDraftDirty, setRawDraftDirty] = useState(false);
  // 固定 provider 名单/API 契约由后端 preview 下发，前端不硬编码。
  const [fixedProviders, setFixedProviders] = useState<FixedProviderSet>(() => makeFixedProviderSet([]));
  // 二次勾选：按供应商/渠道分组的发布选择（网关 id 粒度），默认只勾选已发布。
  const [groups, setGroups] = useState<PreviewGroup[]>([]);
  const [removedIds, setRemovedIds] = useState<string[]>([]);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());

  const selectionKey = (supplier: string, channel: string, model: string) =>
    `${supplier}/${channel}/${model}`;

  const drafts = useMemo<ModelDraft[]>(() => Object.entries(canonicalDraft ?? {}).flatMap(([providerKey, entry]) => {
    const models = asRecord(entry).models;
    return Array.isArray(models)
      ? models.map((model) => {
          const id = asRecord(model).id;
          return { ...draftFromEntry(model as ModelEntry, `${providerKey}/${String(id ?? "")}`), providerKey };
        })
      : [];
  }), [canonicalDraft]);

  const updateDraft = (next: ModelDraft) => {
    if (!canonicalDraft || !next.providerKey) return;
    const providers = JSON.parse(JSON.stringify(canonicalDraft)) as Record<string, unknown>;
    const provider = asRecord(providers[next.providerKey]);
    const models = Array.isArray(provider.models) ? provider.models : [];
    const index = models.findIndex((model) => asRecord(model).id === next.id);
    if (index < 0) return;
    models[index] = modelPreview(next);
    provider.models = models;
    providers[next.providerKey] = provider;
    setCanonicalDraft(providers);
    setRawText(JSON.stringify({ providers }, null, 2));
    setRawDraftDirty(true);
    setDraftView("proposed");
  };

  const applyPreview = (
    preview: GatewayPreview,
    checkedOverride?: Set<string>,
    view: "current" | "proposed" = "proposed",
  ) => {
    // Backend owns the fixed-provider contract; keep this preview's copy in
    // state so validation/filtering use exactly what the backend declared.
    const fixed = makeFixedProviderSet(preview.fixed_providers);
    setFixedProviders(fixed);
    const cur = filterFixedGatewayProviders(asRecord(preview.current), fixed);
    const prop = filterFixedGatewayProviders(asRecord(preview.proposed), fixed);
    const groupsFromServer = preview.groups;
    const removedArr = preview.removed ?? [];
    const nextProposed = prop ?? {};
    const nextDisplayed = view === "current" ? cur ?? {} : nextProposed;
    setCurrent(cur);
    if (view === "current") setFullProposedDraft(nextProposed);
    setProposedDraft(nextProposed);
    setDraftView(view);
    setCanonicalDraft(nextDisplayed);
    setRawText(JSON.stringify({ providers: nextDisplayed }, null, 2));
    setRawDraftDirty(false);
    setConflicts(preview.conflicts);
    // UI only reads the fixed gateway set: drop wild third-party keys from the
    // displayed diff. pending_count/conflicts stay backend-canonical (wild is
    // never counted by the backend for these fixed providers).
    const diff = filterFixedGatewayDiff(preview.diff, fixed);
    setBackendDiff(diff);
    setBackendPending(preview.pending_count);
    setGroups(groupsFromServer);
    setRemovedIds(removedArr);


    const knownIds = new Set(
      groupsFromServer.flatMap((group) =>
        group.models.map((model) => selectionKey(group.supplier, group.channel, model.id)),
      ),
    );
    const persistedUnchecked = loadUncheckedIds(knownIds);
    const publishedIds = new Set(
      groupsFromServer
        .filter((group) => group.models.some((model) => model.status === "published"))
        .flatMap((group) =>
          group.models
            .filter((model) => model.status === "published")
            .map((model) => selectionKey(group.supplier, group.channel, model.id)),
        ),
    );
    const nextChecked = checkedOverride ?? publishedIds;
    const filteredChecked = new Set(
      [...nextChecked].filter((id) => !persistedUnchecked.has(id)),
    );
    setChecked(filteredChecked);
    skipAutoCheck.current.clear();
    for (const id of persistedUnchecked) skipAutoCheck.current.add(id);
  };

  const load = async (selection?: Set<string>) => {
    setLoading(true);
    try {
      const draft = !rawDraftDirty && fullProposedDraft
        ? { providers: fullProposedDraft }
        : rawValidation.ok && rawValidation.value
          ? rawValidation.value
          : undefined;
      const preview = selection === undefined
        ? await api.previewGateway()
        : await api.previewGateway({
            selected: selectedGatewayModels(selection),
            draft,
          });
      applyPreview(preview, selection, "current");
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const [rawText, setRawText] = useState("{}");
  const rawValidation = useMemo(
    () => validateGatewayJson(rawText, fixedProviders),
    [rawText, fixedProviders],
  );
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
  function handleRawChange(text: string) {
    setRawText(text);
    setRawDraftDirty(true);
    const parsed = validateGatewayJson(text, fixedProviders);
    if (!parsed.ok || !parsed.value) return;
		const providers = asRecord(parsed.value.providers);
		setCanonicalDraft(providers);
  }
  function formatRaw() {
    try {
      setRawText(JSON.stringify(JSON.parse(rawText), null, 2));
    } catch {}
  }

  // diff for status bar: Current vs Proposed (backend) – pending publish count
  const statusDiff = backendDiff;
  const pendingCount = backendPending;

  // 用户取消勾选的 source selection 跨 preview reload 保留。
  const skipAutoCheck = useRef<Set<string>>(new Set());

  function providerModelKey(g: PreviewGroup, itemId: string): string {
    return selectionKey(g.supplier, g.channel, itemId);
  }
  function displayGatewayId(_g: PreviewGroup, itemId: string): string {
    return itemId;
  }

  function selectedGatewayModels(selection: Set<string>): GatewaySelection[] {
    return groups.flatMap((group) =>
      group.models
        .filter((model) => selection.has(selectionKey(group.supplier, group.channel, model.id)))
        .map((model) => ({
          supplier: group.supplier,
          channel: group.channel,
          model: model.id,
        })),
    );
  }

  async function toggleChecked(id: string) {
    const next = new Set(checked);
    if (next.has(id)) {
      next.delete(id);
      skipAutoCheck.current.add(id);
      addUncheckedId(id);
    } else {
      next.add(id);
      skipAutoCheck.current.delete(id);
      removeUncheckedId(id);
    }
    setChecked(next);
    try {
      const previewInput: { selected: GatewaySelection[]; draft?: unknown } = {
        selected: selectedGatewayModels(next),
      };
      if (rawDraftDirty && rawValidation.ok && rawValidation.value) {
        previewInput.draft = rawValidation.value;
      } else if (fullProposedDraft) {
        previewInput.draft = { providers: fullProposedDraft };
      }
      const preview = await api.previewGateway(previewInput);
      applyPreview(preview, next, "proposed");
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    }
  }

  async function handleApplyToPi() {
    try {
      if (!rawValidation.ok || !rawValidation.value) {
        throw new Error(rawValidation.error ?? "Invalid JSON");
      }
      const selection = !rawDraftDirty && groups.length > 0 ? checked : undefined;
      const hasCurrentGatewayModels = Object.entries(current ?? {}).some(([providerKey, entry]) => {
        if (!isFixedGatewayProvider(providerKey, fixedProviders)) return false;
        const models = asRecord(entry).models;
        return Array.isArray(models) && models.length > 0;
      });
      if (selection && selection.size === 0 && groups.some((group) => group.models.length > 0) && !hasCurrentGatewayModels) {
        throw new Error("请先勾选至少一个模型");
      }
      const draft = !rawDraftDirty && proposedDraft
        ? { providers: proposedDraft }
        : rawValidation.value;
      const previewInput = selection === undefined
        ? { draft }
        : { selected: selectedGatewayModels(selection), draft };
      const preview = await api.previewGateway(previewInput);
      const fixed = makeFixedProviderSet(preview.fixed_providers);
      setFixedProviders(fixed);
      if (preview.conflicts.length > 0) {
        setConflicts(preview.conflicts);
        setBackendPending(preview.pending_count);
        setBackendDiff(filterFixedGatewayDiff(preview.diff, fixed));
        throw new Error(preview.conflicts.join("; "));
      }
      await api.applyGateway({ providers: preview.proposed });
      applyPreview(preview);
      const now = new Date().toISOString();
      try { if (typeof window !== "undefined") window.localStorage?.setItem(LAST_PUBLISH_KEY, now); } catch {}
      setLastPublishAt(now);
      toast("ok", t("Saved") || "Saved");
      await mutateAfterGatewayPublish();
      // Reload current plus the full proposal so a later selection can add another model.
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
    if (!current || Object.keys(current).length === 0) return "尚未发布";
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
            <span className="text-zinc-400">勾选子集待发布：{pendingCount}</span>
          </div>
          <div className="space-y-2">
            {groups.map((g) => {
              const sourceTitle = g.channel ? `${g.supplier} / ${g.channel}` : g.supplier;
              const title = g.gatewayProvider ? `${g.gatewayProvider} · ${sourceTitle}` : sourceTitle;
              const pub = g.models.filter((m) => m.status === "published").length;
              return (
                <div key={`${g.gatewayProvider}/${g.supplier}/${g.channel}`} className="rounded border border-white/10 px-2 py-1">
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
                  <div key={id} className="mt-1 font-mono text-xs text-zinc-400">{id}</div>
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
          </div>
          <div className="mt-1 text-xs text-zinc-500">
            网关模型元信息可直接编辑：显示名称、contextWindow、maxTokens、cost、input、reasoning、headers、compat、extra。
          </div>

          <div className="mt-3 space-y-2">
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
                gatewayOnly
                onChange={updateDraft}
                expanded={expandedKeys.has(d.key)}
                onToggleExpanded={() => setExpandedKeys((current) => {
                  const next = new Set(current);
                  next.has(d.key) ? next.delete(d.key) : next.add(d.key);
                  return next;
                })}
              />
            ))}
          </div>
        </div>
      </Card>
        {/* Live JSON preview — editable */}
        <div className="mt-6">
          <div className="mb-1 text-sm font-medium text-zinc-200">
            {draftView === "current" ? "已落盘 JSON" : "待发布 JSON"}
          </div>
          <div className="space-y-2">
			<JsonEditor value={rawText} onChange={handleRawChange} label="gateway json" className="h-80" errorLine={gatewayErrorLine} />
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
                onClick={() => void handleApplyToPi()}
              >
                应用到 Pi
              </Button>
            </div>
          </div>
        </div>
    </div>
  );
}

import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { Button, Card, Field, Input, Select, SectionTitle } from "./ui";
import { useI18n } from "../i18n";
import { useAction, useToast } from "./ui";
import { ModelCard } from "./ModelCard";
import { RequestHeadersEditor } from "./RequestHeadersEditor";
import { StructuredOptionsEditor } from "./StructuredOptionsEditor";
import { draftFromEntry, modelPreview, newModelDraft, type ModelDraft } from "../lib/piModel";
import { diffGateway, validateGatewayJson } from "../lib/gatewayDiff";
import type { ModelEntry } from "../types";
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
  const [headers, setHeaders] = useState<Record<string, string>>({});
  const [compat, setCompat] = useState<Record<string, unknown>>({});
  const [apiType, setApiType] = useState("openai-completions");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [lastPublishAt, setLastPublishAt] = useState<string | null>(() => {
    try { return typeof window !== "undefined" ? window.localStorage?.getItem(LAST_PUBLISH_KEY) ?? null : null; } catch { return null; }
  });
  const [showMismatchBanner, setShowMismatchBanner] = useState(false);
  const [hasCheckedMismatch, setHasCheckedMismatch] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const preview = await api.previewGateway();
      const cur = (preview as any).current as Record<string, unknown> | null;
      const prop = (preview as any).proposed as Record<string, unknown>;
      const conf = (preview as any).conflicts as string[] ?? [];
      setCurrent(cur);
      setProposed(prop);
      setConflicts(conf);
      const src = prop ?? cur ?? {};
      setDraft(src);
      const rec = asRecord(src);
      setApiType((rec.api as string) || "openai-completions");
      setBaseUrl((rec.baseUrl as string) || "");
      setApiKey((rec.apiKey as string) || "");
      setHeaders(
        rec.headers && typeof rec.headers === "object" && !Array.isArray(rec.headers)
          ? (rec.headers as Record<string, string>)
          : {},
      );
      setCompat(
        rec.compat && typeof rec.compat === "object" && !Array.isArray(rec.compat)
          ? (rec.compat as Record<string, unknown>)
          : {},
      );
      const models = Array.isArray(rec.models) ? (rec.models as unknown[]) : [];
      setDrafts(models.map((m) => draftFromEntry(m as ModelEntry)));
      // 首次进入若 preview diff 非空，顶部提示是否立即同步，默认不自动写
      if (!hasCheckedMismatch) {
        const curForDiff = cur as Record<string, unknown> | null;
        const propForDiff = prop as Record<string, unknown>;
        if (propForDiff) {
          const d = diffGateway(curForDiff, propForDiff);
          const hasDiff = d.added.length > 0 || d.removed.length > 0 || d.changed.length > 0;
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
      ...(Object.keys(headers).length ? { headers } : { headers: undefined }),
      ...(Object.keys(compat).length ? { compat } : { compat: undefined }),
      models: modelsPreview,
    };
    if (!Object.keys(headers).length) delete (next as any).headers;
    if (!Object.keys(compat).length) delete (next as any).compat;
    if (!apiKey) delete (next as any).apiKey;
    return JSON.stringify(next, null, 2);
  }, [draft, drafts, apiType, baseUrl, apiKey, headers, compat]);

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
      setHeaders(rec.headers && typeof rec.headers === "object" && !Array.isArray(rec.headers) ? (rec.headers as Record<string, string>) : {});
      setCompat(rec.compat && typeof rec.compat === "object" && !Array.isArray(rec.compat) ? (rec.compat as Record<string, unknown>) : {});
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

  const pendingCount = statusDiff.added.length + statusDiff.removed.length + statusDiff.changed.length;

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

  async function handleApplyToPi() {
    const activeValidation = rawValidation;
    if (!activeValidation.ok || !activeValidation.value) {
      toast("err", activeValidation.error ?? "Invalid JSON");
      return;
    }
    try {
      await api.applyGateway(activeValidation.value);
      const now = new Date().toISOString();
      try { if (typeof window !== "undefined") window.localStorage?.setItem(LAST_PUBLISH_KEY, now); } catch {}
      setLastPublishAt(now);
      setShowMismatchBanner(false);
      toast("ok", t("Saved") || "Saved");
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
          </Field>
          <Field label={t("Base URL")}>
            <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://api.example.com/v1" />
          </Field>
          <div className="sm:col-span-2">
            <Field label={t("API key")}>
              <Input value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-…" />
            </Field>
          </div>
        </div>

        <div className="mt-4 space-y-4">
          <RequestHeadersEditor headers={headers} onHeadersChange={setHeaders} />
          <StructuredOptionsEditor
            title={t("Compatibility")}
            hint={t("Adjust compatibility for endpoints or local services.")}
            emptyLabel={t("No compatibility options")}
            addLabel={t("Add")}
            options={compat}
            onOptionsChange={setCompat}
          />
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
                onToggleExposed={() => {}}
                onChange={(next) => setDrafts((prev) => prev.map((x) => (x.key === d.key ? next : x)))}
                onRemove={() =>
                  setDrafts((prev) => prev.filter((x) => x.key !== d.key))
                }
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

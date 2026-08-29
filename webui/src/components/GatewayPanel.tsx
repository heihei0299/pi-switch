import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { Button, Card, Field, Input, Select, SectionTitle } from "./ui";
import { useI18n } from "../i18n";
import { useAction, useToast } from "./ui";
import { GatewayPreviewModal } from "./GatewayPreviewModal";
import { ModelCard } from "./ModelCard";
import { RequestHeadersEditor } from "./RequestHeadersEditor";
import { StructuredOptionsEditor } from "./StructuredOptionsEditor";
import { draftFromEntry, modelPreview, newModelDraft, type ModelDraft } from "../lib/piModel";
import { validateGatewayJson } from "../lib/gatewayDiff";
import type { ModelEntry } from "../types";

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

export function GatewayPanel({ refresh }: { refresh: () => Promise<void> }) {
  const { t } = useI18n() as any;
  const toast = useToast();
  const run = useAction();
  const [current, setCurrent] = useState<Record<string, unknown> | null>(null);
  const [draft, setDraft] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(true);
  const [drafts, setDrafts] = useState<ModelDraft[]>([]);
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [headers, setHeaders] = useState<Record<string, string>>({});
  const [compat, setCompat] = useState<Record<string, unknown>>({});
  const [apiType, setApiType] = useState("openai-completions");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [showPreviewModal, setShowPreviewModal] = useState(false);
  const [previewData, setPreviewData] = useState<{ current: unknown; proposed: unknown; conflicts: string[] } | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const preview = await api.previewGateway();
      const cur = (preview as any).current as Record<string, unknown> | null;
      const prop = (preview as any).proposed as Record<string, unknown>;
      setCurrent(cur);
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
    // remove undefined headers/compat if empty to match cc-switch no-empty-write
    if (!Object.keys(headers).length) delete (next as any).headers;
    if (!Object.keys(compat).length) delete (next as any).compat;
    if (!apiKey) delete (next as any).apiKey;
    return JSON.stringify(next, null, 2);
  }, [draft, drafts, apiType, baseUrl, apiKey, headers, compat]);

  const validation = useMemo(() => validateGatewayJson(liveJson), [liveJson]);

  function updateDraftField(key: string, value: unknown) {
    setDraft((prev) => {
      if (!prev) return prev;
      const next = { ...prev, [key]: value };
      return next;
    });
  }

  function addModel() {
    const d = newModelDraft();
    setDrafts((prev) => [...prev, d]);
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.add(d.key);
      return ns;
    });
  }

  async function handleFetchPreview() {
    try {
      const p = await api.previewGateway();
      setPreviewData(p as any);
      setShowPreviewModal(true);
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    }
  }

  async function handleApply() {
    if (!validation.ok || !validation.value) {
      toast("err", validation.error ?? "Invalid JSON");
      return;
    }
    try {
      await api.applyGateway(validation.value);
      toast("ok", t("Saved") || "Saved");
      await load();
      await refresh();
    } catch (e) {
      toast("err", e instanceof Error ? e.message : String(e));
    }
  }

  async function handleModalConfirm(edited: unknown) {
    await api.applyGateway(edited);
    setShowPreviewModal(false);
    setPreviewData(null);
    toast("ok", t("Saved") || "Saved");
    await load();
    await refresh();
  }

  if (loading) return <div className="text-sm text-zinc-500">{t("Loading…")}</div>;

  const tOr = (k: string, fallback: string) => (t(k) !== k ? t(k) : fallback);

  return (
    <div>
      <SectionTitle hint={t("Provider config injected into ./pi/agent/models.json") || "Gateway injection"}>
        {t("Gateway") || "Gateway"}
      </SectionTitle>

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
              <Button
                type="button"
                onClick={() => void handleFetchPreview()}
                className="h-8"
              >
                {t("Fetch models")}
              </Button>
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

        {/* Live JSON preview — cc-switch 配置 JSON 区域 */}
        <div className="mt-6">
          <div className="mb-1 text-sm font-medium text-zinc-200">{t("Config JSON")}</div>
          <div className="rounded-lg border border-white/10 bg-zinc-950/60 p-3">
            <pre className="max-h-80 overflow-auto font-mono text-xs text-zinc-300">{liveJson}</pre>
            {!validation.ok && (
              <div className="mt-2 rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">
                Invalid JSON: {validation.error}
              </div>
            )}
            {validation.ok && <div className="mt-2 text-xs text-emerald-400">✓ JSON valid</div>}
          </div>
          <div className="mt-3 flex justify-end gap-2">
            <Button onClick={() => void load()}>{t("Cancel")}</Button>
            <Button
              variant="primary"
              disabled={!validation.ok}
              onClick={() => void run(() => handleApply(), t("Saved") || "Saved")}
            >
              {t("Save") || "Save"}
            </Button>
          </div>
        </div>
      </Card>

      {showPreviewModal && previewData && (
        <GatewayPreviewModal
          current={previewData.current as any}
          proposed={previewData.proposed as any}
          conflicts={previewData.conflicts}
          onClose={() => {
            setShowPreviewModal(false);
            setPreviewData(null);
          }}
          onConfirm={(edited) => run(() => handleModalConfirm(edited), t("Saved") || "Saved")}
        />
      )}
    </div>
  );
}

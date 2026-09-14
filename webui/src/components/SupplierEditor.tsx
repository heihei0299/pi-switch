import { useMemo, useState } from "react";
import type { AppState, ModelEntry, PresetInfo, ProtocolApiCapability, ProviderProfile, ResponsesMode } from "../types";
import { responsesModeError } from "../lib/responsesMode";
import { allowedResponsesModes, defaultProtocolApiId, protocolApiIds } from "../lib/protocolCapabilities";
import { useProtocolCapabilities } from "../lib/protocolContext";
import { validateProfileJson } from "../lib/piModel";
import { materializeProfileForSave, useProfileDraft } from "../hooks/useProfileDraft";
import { JsonEditor } from "./JsonEditor";
import { api } from "../api";
import { useI18n } from "../i18n";
import { Button, Field, Input, Modal, Select, Textarea, useAction, useToast } from "./ui";
import { StructuredOptionsEditor } from "./StructuredOptionsEditor";
import { useDebounce } from "../hooks/useDebounce";
import { mergePreviewHeaders } from "../lib/previewHeaders";
import { mutateAfterProfilePut } from "../store/swr";
import { ChannelEditor, apiTypeOptions, type UpstreamChange, type UpstreamForm } from "./ChannelEditor";

function responsesModeLabel(mode: ResponsesMode, t: (key: string) => string): string {
  if (mode === "passthrough") return `passthrough — ${t("native Responses only")}`;
  if (mode === "convert") return `convert — ${t("Chat Completions only")}`;
  return `auto — ${t("automatic by API type")}`;
}

const SPOOFS = [
  { value: "", label: "none" },
  { value: "claude-code", label: "claude-code" },
  { value: "codex", label: "codex" },
  { value: "gemini", label: "gemini" },
];

function emptyProfile(caps: readonly ProtocolApiCapability[]): ProviderProfile {
  return {
    api: defaultProtocolApiId(caps),
    responsesMode: "auto",
    baseUrl: "",
    apiKey: "",
    proxy: false,
    headers: {},
    compat: {},
    upstreams: [],
  };
}

export function SupplierEditor({
  state,
  original,
  onClose,
  onSaved,
}: {
  state: AppState;
  original: string | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const run = useAction();
  const toast = useToast();
  const { t } = useI18n() as any;
  const caps = useProtocolCapabilities();
  const existing = original ? state.profiles[original] : undefined;
  const presets = usePresets();

  const [name, setName] = useState(original ?? "");
  const draft = useProfileDraft(existing ?? emptyProfile(caps));
  const value = draft.state.value;
  const apiType = value.api;
  const responsesMode = value.responsesMode ?? "auto";
  const baseUrl = value.baseUrl;
  const apiKey = value.apiKey;
  const spoof = value.userAgent ?? "";
  const proxy = value.proxy;
  const preset = value.preset ?? "";
  const modelsDevProvider = value.modelsDevProvider ?? "";
  const headers = value.headers ?? {};
  const compat = value.compat ?? {};
  const upstreams = useMemo<UpstreamForm[]>(
    () => (value.upstreams ?? []).map((upstream, index) => ({
      ...upstream,
      key: `us-${index}`,
      baseUrl: upstream.baseUrl ?? "",
      apiKey: upstream.apiKey ?? "",
      api: upstream.api ?? value.api,
      responsesMode: upstream.responsesMode ?? value.responsesMode ?? "auto",
      weight: upstream.weight != null ? String(upstream.weight) : "",
      name: upstream.name ?? "",
      headers: upstream.headers ?? {},
      models: upstream.models ?? [],
      exposedModels: upstream.exposedModels ?? [],
    })),
    [value.upstreams, value.api, value.responsesMode],
  );
  const legacyModels = ((value as ProviderProfile & { models?: ModelEntry[] }).models ?? []).map((model) => model);
  const modelIds = legacyModels.map((model) => model.id).join("\n");
  const setField = (field: string, fieldValue: unknown) =>
    draft.dispatch({ type: "setProfileField", field, value: fieldValue });
  const setApiType = (next: string) => setField("api", next);
  const setResponsesMode = (next: ResponsesMode) => setField("responsesMode", next);
  const setBaseUrl = (next: string) => setField("baseUrl", next);
  const setApiKey = (next: string) => setField("apiKey", next);
  const setSpoof = (next: string) => setField("userAgent", next || undefined);
  const setProxy = (next: boolean) => setField("proxy", next);
  const setPreset = (next: string) => setField("preset", next || undefined);
  const setModelsDevProvider = (next: string) => setField("modelsDevProvider", next || undefined);
  const setHeaders = (next: Record<string, string>) => setField("headers", next);
  const setCompat = (next: Record<string, unknown>) => setField("compat", next);
  const setUpstreams = (next: UpstreamChange) => {
    const resolved = typeof next === "function" ? next(upstreams) : next;
    draft.dispatch({ type: "setProfileField", field: "upstreams", value: resolved.map(({ key: _key, weight, name: upstreamName, ...upstream }) => ({
      ...upstream,
      name: upstreamName.trim() || undefined,
      weight: weight.trim() ? Number(weight) : undefined,
      headers: Object.keys(upstream.headers ?? {}).length ? upstream.headers : undefined,
    })) });
  };
  const setModelIds = (next: string) => {
    const ids = next.split(/[\n,]/).map((id) => id.trim()).filter(Boolean);
    const existingById = new Map(legacyModels.map((model) => [model.id, model]));
    setField("models", ids.map((id) => existingById.get(id) ?? ({ id } as ModelEntry)));
  };
  const debouncedSpoof = useDebounce(spoof, 300);
  const debouncedHeaders = useDebounce(headers, 300);
  const previewHeaders = useMemo(() => mergePreviewHeaders(debouncedHeaders, upstreams[0]?.headers), [debouncedHeaders, upstreams]);
  const previewUpstreamAggregated = useMemo(() => {
    const firstHeaders = upstreams[0]?.headers;
    return mergePreviewHeaders(headers, firstHeaders);
  }, [headers, upstreams]);
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const text = draft.state.rawText;
  const setText = (next: string) => draft.dispatch({ type: "setRawText", text: next });
  const jsonValidation = useMemo(() => validateProfileJson(text, protocolApiIds(caps)), [text, caps]);
  const profileErrorLine = useMemo(() => {
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
  function switchToRaw() {
    setMode("raw");
  }
  function switchToStructured() {
    if (!jsonValidation.ok || !jsonValidation.value) return;
    setMode("structured");
  }
  function formatProfileJson() {
    try {
      setText(JSON.stringify(JSON.parse(text), null, 2));
    } catch {}
  }

  function applyPreset(id: string) {
    setPreset(id);
    const p = presets.find((x) => x.id === id);
    if (!p) return;
    setApiType(p.api);
    setBaseUrl(p.baseUrl);
    if (!modelIds.trim()) setModelIds(p.models.join("\n"));
  }

  async function saveLocal() {
    const trimmed = name.trim();
    if (!trimmed) throw new Error(t("name required"));
    if (mode === "raw" && (!jsonValidation.ok || !jsonValidation.value)) {
      throw new Error(jsonValidation.error ?? "Invalid JSON");
    }
    const modeError = responsesModeError(value.api, value.responsesMode ?? "auto", caps);
    if (modeError) throw new Error(t(modeError));
    const profile = materializeProfileForSave(draft.state.value);
    profile.updatedAt = new Date().toISOString();
    // 渠道名称前端先行校验：与后端 isValidChannelName 同口径，避免整单 400 才暴露问题。
    const rule = /^[A-Za-z0-9-_]{1,32}$/;
    const seenCh = new Set<string>();
    for (const u of profile.upstreams ?? []) {
      const n = (u.name ?? "").trim();
      if (!rule.test(n)) throw new Error(`渠道名称必填且仅限字母数字/-/_（1-32字符），当前：${n || "(空)"}`);
      if (seenCh.has(n)) throw new Error(`渠道名称重复：${n}`);
      seenCh.add(n);
    }
    if (original) {
      await api.updateProfile(trimmed, profile, original !== trimmed ? original : undefined);
    } else {
      await api.addProfile(trimmed, profile);
    }
    try { await api.getState(); } catch {}
    await mutateAfterProfilePut();
    toast("ok", "已保存到本地，需到网关发布");
    await onSaved();
  }

  return (
    <>
      <Modal title={original ? `${t("Edit")} ${original}` : t("Add profile")} onClose={onClose} wide>
        <Field label={t("Name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="my-provider" />
        </Field>
        {mode === "structured" ? (
          <div className="grid gap-x-4 sm:grid-cols-2">
            <Field label={t("Preset (prefill)")}>
              <Select value={preset} onChange={(e) => applyPreset(e.target.value)}>
                <option value="">— {t("none")} —</option>
                {presets.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label={t("模型目录 Provider")}>
              <Input
                value={modelsDevProvider}
                onChange={(e) => setModelsDevProvider(e.target.value)}
                placeholder="如 openai/anthropic/deepseek，留空按 preset 推断"
              />
            </Field>
            <Field label={t("API type")}>
              <Select value={apiType} onChange={(e) => setApiType(e.target.value)}>
                {apiTypeOptions(caps).map((o) => (
                  <option key={o.value} value={o.value} disabled={o.disabled}>
                    {o.label}
                  </option>
                ))}
                {!protocolApiIds(caps).includes(apiType) && apiType && (
                  // 旧配置里的未知值仍要能显示并回显，但不可再选：服务端写入口会拒绝它。
                  <option value={apiType} disabled>
                    {apiType}
                  </option>
                )}
              </Select>
              <p className="mt-1 text-xs text-zinc-500">{t("Select the API interface format for the AI service.")}</p>
            </Field>
            <Field label={t("Responses mode")}>
              <Select value={responsesMode} onChange={(e) => setResponsesMode(e.target.value as ResponsesMode)}>
                {allowedResponsesModes(caps, apiType).map((mode) => (
                  <option key={mode} value={mode}>{responsesModeLabel(mode, t)}</option>
                ))}
                {!allowedResponsesModes(caps, apiType).includes(responsesMode) && (
                  // 与 api 选择器同一原则：旧值可见可回显，但不能被重新选中。
                  <option value={responsesMode} disabled>
                    {responsesModeLabel(responsesMode, t)}
                  </option>
                )}
              </Select>
            </Field>
            <Field label={t("Disguise (User-Agent)")}>
              <Select value={spoof} onChange={(e) => setSpoof(e.target.value)}>
                {SPOOFS.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </Select>
            </Field>
            <ChannelEditor
              apiType={apiType}
              responsesMode={responsesMode}
              caps={caps}
              baseUrl={baseUrl}
              apiKey={apiKey}
              headers={headers}
              upstreams={upstreams}
              debouncedSpoof={debouncedSpoof}
              previewHeaders={previewHeaders}
              previewUpstreamAggregated={previewUpstreamAggregated}
              onBaseUrlChange={setBaseUrl}
              onApiKeyChange={setApiKey}
              onHeadersChange={setHeaders}
              onUpstreamsChange={setUpstreams}
            />
            <div className="sm:col-span-2">
              <StructuredOptionsEditor
                title={t("Compatibility") !== "接口兼容性" ? t("Compatibility") : "接口兼容性"}
                hint={t("Adjust compatibility for endpoints or local services.") !== "调整兼容端点或本地服务的请求行为。" ? t("Adjust compatibility for endpoints or local services.") : "调整兼容端点或本地服务的请求行为。"}
                emptyLabel={t("No compatibility options") !== "暂无兼容性选项" ? t("No compatibility options") : "暂无兼容性选项"}
                addLabel={t("Add") !== "添加" ? t("Add") : "添加"}
                options={compat}
                onOptionsChange={setCompat}
              />
            </div>
            <div className="sm:col-span-2">
              <Field label={t("Model IDs (one per line)")}>
                <Textarea
                  rows={4}
                  value={modelIds}
                  onChange={(e) => setModelIds(e.target.value)}
                  placeholder={"gpt-4o\ngpt-4o-mini"}
                />
              </Field>
            </div>
            <label className="mb-3 flex items-center gap-2 text-sm text-zinc-300 sm:col-span-2">
              <input type="checkbox" checked={proxy} onChange={(e) => setProxy(e.target.checked)} />
              {t("Mark as a proxy profile (not exposed to pi)")}
            </label>
          </div>
        ) : (
          <div className="space-y-2">
            <JsonEditor value={text} onChange={setText} label="profile json" className="h-64 sm:h-80" errorLine={profileErrorLine} />
            {!jsonValidation.ok && (
              <div className="rounded border border-red-500/30 bg-red-950/40 px-2 py-1 text-xs text-red-200">Invalid JSON: {jsonValidation.error}</div>
            )}
            {jsonValidation.ok && <div className="text-xs text-emerald-400">✓ JSON valid</div>}
          </div>
        )}
        <div className="mt-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Button onClick={formatProfileJson} className="h-7 text-xs">格式化</Button>
            {mode === "structured" ? (
              <Button onClick={() => switchToRaw()} className="h-7 text-xs">JSON</Button>
            ) : (
              <Button onClick={() => switchToStructured()} className="h-7 text-xs">结构化</Button>
            )}
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

let presetCache: PresetInfo[] | null = null;
function usePresets(): PresetInfo[] {
  const [presets, setPresets] = useState<PresetInfo[]>(presetCache ?? []);
  useMemo(() => {
    if (presetCache) return;
    api
      .getPresets()
      .then((p) => {
        presetCache = p;
        setPresets(p);
      })
      .catch(() => {});
  }, []);
  return presets;
}

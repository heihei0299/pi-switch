import { useEffect, useMemo, useState } from "react";
import type { AppState, CcsProvider, ModelEntry, PresetInfo, ProviderProfile, ResponsesMode, Upstream } from "../types";
import { hasUpstreams, resolvedUpstreams } from "../types";
import { effectiveResponsesMode, responsesModeError } from "../lib/responsesMode";
import { draftFromEntry, modelPreview, newModelDraft, validateModelsJson, validateProfileJson, type ModelDraft } from "../lib/piModel";
import { JsonEditor } from "./JsonEditor";
import { api } from "../api";
import { useI18n } from "../i18n";
import {
  Badge,
  Button,
  Card,
  Field,
  Input,
  Modal,
  SectionTitle,
  Select,
  Textarea,
  useAction,
  useToast,
  cx,
} from "./ui";
import { ModelCard } from "./ModelCard";
import { SupplierCreditsPanel } from "./SupplierCreditsPanel";
import { RequestHeadersEditor } from "./RequestHeadersEditor";
import { StructuredOptionsEditor } from "./StructuredOptionsEditor";
const API_TYPE_OPTIONS: ReadonlyArray<{ value: string; label: string }> = [
  { value: "openai-completions", label: "OpenAI Chat Completions" },
  { value: "openai-responses", label: "OpenAI Responses" },
  { value: "anthropic-messages", label: "Anthropic Messages" },
  { value: "google-generative-ai", label: "Google Gemini" },
];
const API_TYPES = API_TYPE_OPTIONS.map((o) => o.value);
const SPOOFS = [
  { value: "", label: "none" },
  { value: "claude-code", label: "claude-code" },
  { value: "codex", label: "codex" },
  { value: "gemini", label: "gemini" },
];

function defaultModel(id: string): ModelEntry {
  return { id, input: ["text"], contextWindow: 128000, maxTokens: 16384 };
}

export function ProfilesPanel({
  state,
  refresh,
}: {
  state: AppState;
  refresh: () => Promise<void>;
}) {
  const run = useAction();
  const toast = useToast();
  const { t, lang } = useI18n() as any;
  const [editing, setEditing] = useState<{ name: string | null } | null>(null);
  const [models, setModels] = useState<string | null>(null); // profile name for models modal
  const [ccImport, setCcImport] = useState(false);

  const entries = Object.entries(state.profiles).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div>
      <SectionTitle hint={`${entries.length} ${t("profile(s)")}`}>{t("Profiles")}</SectionTitle>

      <div className="mb-3 flex gap-2">
        <Button variant="primary" onClick={() => setEditing({ name: null })}>
          {t("+ Add profile")}
        </Button>
        <Button onClick={() => setCcImport(true)}>⇥ {t("Import from cc-switch")}</Button>
      </div>

      <div className="space-y-2">
        {entries.length === 0 && (
          <Card>
            <div className="text-sm text-zinc-500">
              {t("No profiles yet.")} {t('Add one with the "+ Add profile" button or import from cc-switch.')}
            </div>
          </Card>
        )}
        {entries.map(([name, p]) => {
          const isCurrent = state.current === name;
          const exposed = p.exposedModels?.length ?? 0;
          return (
            <Card key={name} className="flex flex-col gap-3">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="truncate font-medium text-zinc-100">{name}</span>
                  {isCurrent && <Badge tone="amber">{t("current")}</Badge>}
                  {p.proxy && <Badge tone="amber">{t("proxy")}</Badge>}
                  <Badge>{p.api || "?"}</Badge>
                  <Badge tone="amber">{t("Responses")}: {effectiveResponsesMode(p)}</Badge>
                  {exposed > 0 && <Badge tone="green">{exposed} {t("exposed")}</Badge>}
                </div>
                <div className="mt-0.5 truncate text-xs text-zinc-500">
                  {(hasUpstreams(p) ? resolvedUpstreams(p)[0]?.baseUrl : p.baseUrl) || t("no base url")} · {p.models?.length ?? 0} {t("models")} {hasUpstreams(p) ? `· ${resolvedUpstreams(p).length} upstream(s)` : ""}
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-1.5 lg:shrink-0 lg:justify-end">
                {!isCurrent && (
                  <Button
                    onClick={() =>
                      run(() => api.useProfile(name), `${t("Switched to")} ${name}`, refresh)
                    }
                  >
                    {t("Use")}
                  </Button>
                )}
                <Button onClick={() => setModels(name)}>{t("Models")}</Button>
                <Button onClick={() => setEditing({ name })}>{t("Edit")}</Button>
                <ProfileCardMenu
                  name={name}
                  onTest={() =>
                    run(
                      async () => {
                        const r = await api.testProfile(name);
                        if (!r.success) throw new Error(r.message);
                        return r;
                      },
                      t("Test OK"),
                    )
                  }
                  onCopy={() => {
                    const to = prompt(
                      `${t("Duplicate profile '{{name}}' as:").replace("{{name}}", name)}`,
                      `${name}-copy`,
                    );
                    if (to) run(() => api.duplicateProfile(name, to), t("Duplicated"), refresh);
                  }}
                  onDelete={() => {
                    if (confirm(t("Delete profile '{{name}}'?").replace("{{name}}", name)))
                      run(() => api.deleteProfile(name), t("Deleted"), refresh);
                  }}
                />
              </div>
              </div>
              <SupplierCreditsPanel name={name} profile={p} />
            </Card>
          );
        })}
      </div>

      {editing && (
        <ProfileForm
          state={state}
          original={editing.name}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null);
            await refresh();
          }}
        />
      )}

      {models && (
        <ModelsModal
          name={models}
          profile={state.profiles[models]}
          onClose={() => setModels(null)}
          onSaved={async () => {
            setModels(null);
            await refresh();
          }}
        />
      )}

      {ccImport && (
        <CcsImportModal
          onClose={() => setCcImport(false)}
          onImported={async () => {
            setCcImport(false);
            await refresh();
          }}
        />
      )}
    </div>
  );
}

// ─── Add / Edit form ──────────────────────────────────────

function ProfileForm({
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
  const { t, lang } = useI18n() as any;
  const existing = original ? state.profiles[original] : undefined;
  const presets = usePresets();

  const [name, setName] = useState(original ?? "");
  const [apiType, setApiType] = useState(existing?.api ?? "openai-completions");
  const [responsesMode, setResponsesMode] = useState<ResponsesMode>(existing?.responsesMode ?? "auto");
  const [baseUrl, setBaseUrl] = useState(existing?.baseUrl ?? "");
  const [apiKey, setApiKey] = useState(existing?.apiKey ?? "");
  const [spoof, setSpoof] = useState(existing?.userAgent ?? "");
  const [proxy, setProxy] = useState(existing?.proxy ?? false);
  const [preset, setPreset] = useState(existing?.preset ?? "");
  const [modelsDevProvider, setModelsDevProvider] = useState(existing?.modelsDevProvider ?? "");
  const [headers, setHeaders] = useState<Record<string, string>>(() => {
    const h = (existing as any)?.headers;
    if (h && typeof h === "object" && !Array.isArray(h)) return h as Record<string, string>;
    return {};
  });
  const [compat, setCompat] = useState<Record<string, unknown>>(() => {
    const c = (existing as any)?.compat;
    if (c && typeof c === "object" && !Array.isArray(c)) return c as Record<string, unknown>;
    return {};
  });
  // Upstream 列表：基于 has_upstreams/resolved_upstreams 回退，单字段兼容
  const [upstreams, setUpstreams] = useState<Array<{ key: string; baseUrl: string; apiKey: string; weight: string; name: string; headers: Record<string, string> }>>(() => {
    const existingUps = (existing as any)?.upstreams as Upstream[] | undefined;
    if (existingUps && existingUps.length > 0) {
      return existingUps.map((u, idx) => ({
        key: `us-${idx}-${u.baseUrl.slice(0, 8)}`,
        baseUrl: u.baseUrl ?? "",
        apiKey: u.apiKey ?? "",
        weight: u.weight != null ? String(u.weight) : "",
        name: u.name ?? "",
        headers: (u.headers as Record<string, string>) ?? {},
      }));
    }
    return [];
  });
  const [modelIds, setModelIds] = useState(
    (existing?.models ?? []).map((m) => m.id).join("\n"),
  );
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const [text, setText] = useState<string>(() => {
    const preview: Record<string, unknown> = {
      api: existing?.api ?? "openai-completions",
      responsesMode: existing?.responsesMode ?? "auto",
      baseUrl: existing?.baseUrl ?? "",
      apiKey: existing?.apiKey ?? "",
      ...(existing?.headers && Object.keys(existing.headers as any).length ? { headers: existing.headers } : {}),
      ...(existing?.compat && Object.keys(existing.compat as any).length ? { compat: existing.compat } : {}),
      ...(existing?.upstreams ? { upstreams: existing.upstreams } : {}),
      ...(existing?.proxy ? { proxy: existing.proxy } : {}),
      ...(existing?.preset ? { preset: existing.preset } : {}),
      ...(existing?.modelsDevProvider ? { modelsDevProvider: existing.modelsDevProvider } : {}),
      ...(existing?.userAgent ? { userAgent: existing.userAgent } : {}),
    };
    try {
      return JSON.stringify(preview, null, 2);
    } catch {
      return "{}";
    }
  });
  const jsonValidation = useMemo(() => validateProfileJson(text), [text]);
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
    const preview: Record<string, unknown> = {
      api: apiType,
      responsesMode,
      baseUrl: baseUrl.trim(),
      apiKey: apiKey.trim(),
      ...(Object.keys(headers).length ? { headers } : {}),
      ...(Object.keys(compat).length ? { compat } : {}),
      ...(upstreams.length ? { upstreams: upstreams.map((u) => ({ baseUrl: u.baseUrl.trim(), apiKey: u.apiKey.trim(), headers: Object.keys(u.headers).length ? u.headers : undefined, weight: u.weight.trim() ? Number(u.weight) : undefined, name: u.name.trim() || undefined })) } : {}),
      ...(proxy ? { proxy } : {}),
      ...(preset ? { preset } : {}),
      ...(modelsDevProvider.trim() ? { modelsDevProvider: modelsDevProvider.trim() } : {}),
      ...(spoof ? { userAgent: spoof } : {}),
    };
    try { setText(JSON.stringify(preview, null, 2)); } catch { setText("{}"); }
    setMode("raw");
  }
  function switchToStructured() {
    if (!jsonValidation.ok || !jsonValidation.value) return;
    const v = jsonValidation.value as Record<string, unknown>;
    if (typeof v.api === "string") setApiType(v.api);
    if (typeof v.responsesMode === "string") setResponsesMode(v.responsesMode as ResponsesMode);
    if (typeof v.baseUrl === "string") setBaseUrl(v.baseUrl);
    if (typeof v.apiKey === "string") setApiKey(v.apiKey);
    if (v.headers && typeof v.headers === "object" && !Array.isArray(v.headers)) setHeaders(v.headers as Record<string, string>);
    else if (!v.headers) setHeaders({});
    if (v.compat && typeof v.compat === "object" && !Array.isArray(v.compat)) setCompat(v.compat as Record<string, unknown>);
    else if (!v.compat) setCompat({});
    if (Array.isArray(v.upstreams)) {
      setUpstreams((v.upstreams as Upstream[]).map((u, idx) => ({ key: `us-${idx}-${String(u.baseUrl).slice(0,8)}`, baseUrl: (u as any).baseUrl ?? "", apiKey: (u as any).apiKey ?? "", weight: (u as any).weight != null ? String((u as any).weight) : "", name: (u as any).name ?? "", headers: (u as any).headers ?? {} })));
    } else if (!v.upstreams) setUpstreams([]);
    if (typeof v.proxy === "boolean") setProxy(v.proxy);
    if (typeof v.preset === "string") setPreset(v.preset);
    else if (!v.preset) setPreset("");
    if (typeof v.modelsDevProvider === "string") setModelsDevProvider(v.modelsDevProvider);
    else if (!v.modelsDevProvider) setModelsDevProvider("");
    if (typeof v.userAgent === "string") setSpoof(v.userAgent);
    else if (!v.userAgent) setSpoof("");
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

  function build(): ProviderProfile {
    const ids = modelIds
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean);
    // Preserve existing model metadata by id; default for new ids.
    const prevById = new Map((existing?.models ?? []).map((m) => [m.id, m]));
    const models = ids.map((id) => prevById.get(id) ?? defaultModel(id));
    const exposedModels = (existing?.exposedModels ?? []).filter((id) => ids.includes(id));
    // Upstream 回退：有 upstreams 时持久化多上游，否则回退单字段（兼容旧配置）
    let upstreamPayload: Upstream[] | undefined;
    let effectiveBaseUrl = baseUrl.trim();
    let effectiveApiKey = apiKey.trim();
    let effectiveHeaders: Record<string, string> | undefined = Object.keys(headers).length ? headers : undefined;
    if (upstreams.length > 0) {
      upstreamPayload = upstreams.map((u) => ({
        baseUrl: u.baseUrl.trim(),
        apiKey: u.apiKey.trim(),
        headers: Object.keys(u.headers).length ? u.headers : undefined,
        weight: u.weight.trim() ? Number(u.weight) : undefined,
        name: u.name.trim() || undefined,
      } as Upstream)).filter((u) => u.baseUrl || u.apiKey);
      if (upstreamPayload.length === 0) upstreamPayload = undefined;
      else {
        // 单字段同步首个 upstream，保持 has_upstreams=false 读者兼容
        effectiveBaseUrl = upstreamPayload[0]?.baseUrl ?? effectiveBaseUrl;
        effectiveApiKey = upstreamPayload[0]?.apiKey ?? effectiveApiKey;
        effectiveHeaders = (upstreamPayload[0]?.headers as Record<string, string>) ?? effectiveHeaders;
      }
    }
    return {
      ...(existing ?? {}),
      api: apiType,
      responsesMode,
      baseUrl: effectiveBaseUrl,
      apiKey: effectiveApiKey,
      upstreams: upstreamPayload,
      models,
      proxy,
      exposedModels,
      preset: preset || undefined,
      modelsDevProvider: modelsDevProvider.trim() || undefined,
      userAgent: spoof || undefined,
      headers: effectiveHeaders,
      compat: Object.keys(compat).length ? compat : undefined,
      updatedAt: new Date().toISOString(),
    } as unknown as ProviderProfile;
  }

  async function saveLocal() {
    const trimmed = name.trim();
    if (!trimmed) throw new Error(t("name required"));
    if (mode === "raw") {
      if (!jsonValidation.ok || !jsonValidation.value) throw new Error(jsonValidation.error ?? "Invalid JSON");
      const v = jsonValidation.value as Record<string, unknown>;
      const rawApi = v.api as string;
      const rawMode = (v.responsesMode as ResponsesMode) ?? "auto";
      const modeError2 = responsesModeError(rawApi, rawMode);
      if (modeError2) throw new Error(t(modeError2));
      const profile = {
        ...(existing ?? {}),
        api: rawApi,
        responsesMode: rawMode,
        baseUrl: v.baseUrl as string,
        apiKey: (v.apiKey as string) ?? "",
        upstreams: v.upstreams as Upstream[] | undefined,
        headers: v.headers as Record<string, string> | undefined,
        compat: v.compat as Record<string, unknown> | undefined,
        proxy: (v.proxy as boolean) ?? false,
        preset: v.preset as string | undefined,
        modelsDevProvider: v.modelsDevProvider as string | undefined,
        userAgent: v.userAgent as string | undefined,
        models: existing?.models ?? [],
        exposedModels: existing?.exposedModels ?? [],
        updatedAt: new Date().toISOString(),
      } as unknown as ProviderProfile;
      if (original) {
        await api.updateProfile(trimmed, profile, original !== trimmed ? original : undefined);
      } else {
        await api.addProfile(trimmed, profile);
      }
      toast("ok", "已保存到本地，需到网关发布");
      await onSaved();
      return;
    }
    const modeError = responsesModeError(apiType, responsesMode);
    if (modeError) throw new Error(t(modeError));
    const profile = build();
    if (original) {
      await api.updateProfile(trimmed, profile, original !== trimmed ? original : undefined);
    } else {
      await api.addProfile(trimmed, profile);
    }
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
                {API_TYPE_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
                {!API_TYPES.includes(apiType) && apiType && (
                  <option value={apiType}>{apiType}</option>
                )}
              </Select>
              <p className="mt-1 text-xs text-zinc-500">{t("Select the API interface format for the AI service.")}</p>
            </Field>
            <Field label={t("Responses mode")}>
              <Select value={responsesMode} onChange={(e) => setResponsesMode(e.target.value as ResponsesMode)}>
                <option value="auto">auto — {t("automatic by API type")}</option>
                <option value="passthrough">passthrough — {t("native Responses only")}</option>
                <option value="convert">convert — {t("Chat Completions only")}</option>
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
            {upstreams.length === 0 ? (
              <>
                <div className="sm:col-span-2">
                  <Field label={t("Base URL")}>
                    <Input
                      value={baseUrl}
                      onChange={(e) => setBaseUrl(e.target.value)}
                      placeholder="https://api.example.com/v1"
                    />
                  </Field>
                </div>
                <div className="sm:col-span-2">
                  <Field label={t("API key (supports $ENV_VAR)")}>
                    <Input value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-…" />
                  </Field>
                </div>
                <div className="sm:col-span-2">
                  <RequestHeadersEditor headers={headers} onHeadersChange={setHeaders} />
                </div>
              </>
            ) : null}
            <div className="sm:col-span-2">
              <div className="mb-1 flex items-center justify-between">
                <span className="text-sm font-medium text-zinc-200">{t("Upstreams")} {upstreams.length > 0 ? `· ${upstreams.length}` : `· ${t("single fallback")}`}</span>
                <div className="flex gap-2">
                  {upstreams.length === 0 && (
                    <Button
                      type="button"
                      onClick={() => {
                        const first: any = { key: `us-${Date.now()}`, baseUrl: baseUrl.trim(), apiKey: apiKey.trim(), weight: "", name: "", headers: { ...headers } };
                        setUpstreams([first]);
                      }}
                    >
                      {t("Manage upstreams")}
                    </Button>
                  )}
                  {upstreams.length > 0 && (
                    <>
                      <Button
                        type="button"
                        onClick={() => setUpstreams((prev) => [...prev, { key: `us-${Date.now()}-${prev.length}`, baseUrl: "", apiKey: "", weight: "", name: "", headers: {} }])}
                      >
                        + {t("Add upstream")}
                      </Button>
                      <Button
                        type="button"
                        onClick={() => {
                          if (upstreams.length > 0) {
                            setBaseUrl(upstreams[0].baseUrl);
                            setApiKey(upstreams[0].apiKey);
                            setHeaders(upstreams[0].headers ?? {});
                          }
                          setUpstreams([]);
                        }}
                      >
                        {t("Use single")}
                      </Button>
                    </>
                  )}
                </div>
              </div>
              {upstreams.length > 0 && (
                <div className="space-y-3 rounded-lg border border-white/10 bg-zinc-900/30 p-3">
                  <div className="text-xs text-zinc-500">{t("has_upstreams / resolved_upstreams 回退，多上游为空时使用单 Base URL/API Key。增删即时生效，保存后需到网关发布。")}</div>
                  {upstreams.map((u, idx) => (
                    <div key={u.key} className="rounded-lg border border-white/10 bg-zinc-950 p-3">
                      <div className="mb-2 flex items-center justify-between">
                        <span className="text-xs font-medium text-zinc-300">Upstream #{idx + 1} {u.name ? `· ${u.name}` : ""}</span>
                        <Button type="button" onClick={() => setUpstreams((prev) => prev.filter((x) => x.key !== u.key))} className="h-7 text-xs">{t("Remove")}</Button>
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label={t("Base URL")}>
                          <Input value={u.baseUrl} onChange={(e) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, baseUrl: e.target.value } : x))} placeholder="https://api.example.com/v1" />
                        </Field>
                        <Field label={t("API key")}>
                          <Input value={u.apiKey} onChange={(e) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, apiKey: e.target.value } : x))} placeholder="sk-…" />
                        </Field>
                        <Field label={t("Weight")}>
                          <Input value={u.weight} onChange={(e) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, weight: e.target.value } : x))} placeholder="1" />
                        </Field>
                        <Field label={t("Name")}>
                          <Input value={u.name} onChange={(e) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, name: e.target.value } : x))} placeholder="upstream-a" />
                        </Field>
                      </div>
                      <div className="mt-2">
                        <RequestHeadersEditor headers={u.headers} onHeadersChange={(next) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, headers: next } : x))} />
                      </div>
                    </div>
                  ))}
                  {upstreams.length === 0 && <div className="text-xs text-zinc-500">{t("No upstreams yet.")}</div>}
                </div>
              )}
            </div>
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
              {t("Mark as a proxy profile (excluded from failover, not exposed to pi)")}
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

// ─── Models & expose modal ────────────────────────────────

function ModelsModal({
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
  const [drafts, setDrafts] = useState<ModelDraft[]>(() => (profile.models ?? []).map((m) => draftFromEntry(m)));
  const [exposed, setExposed] = useState<Set<string>>(
    new Set(profile.exposedModels ?? []),
  );
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const [fetching, setFetching] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [mode, setMode] = useState<"structured" | "raw">("structured");
  const [text, setText] = useState<string>(() => {
    try {
      return JSON.stringify((profile.models ?? []).map((m) => modelPreview(draftFromEntry(m as ModelEntry))), null, 2);
    } catch {
      return "[]";
    }
  });
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
    setDrafts((prev) => {
      const old = prev.find((d) => d.key === key);
      // If id changed and old id was exposed, move exposure
      if (old && old.id !== next.id) {
        const oldId = old.id;
        const newId = next.id;
        if (exposed.has(oldId) && newId.trim()) {
          setExposed((s) => {
            const ns = new Set(s);
            ns.delete(oldId);
            ns.add(newId);
            return ns;
          });
        }
      }
      return prev.map((d) => (d.key === key ? next : d));
    });
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
    const prevById = new Map(drafts.map((d) => [d.id, d.key]));
    const nextDrafts = (jsonValidation.value as unknown as ModelEntry[]).map((m) => {
      const id = (m as any).id as string || "";
      const key = prevById.get(id);
      return draftFromEntry(m as ModelEntry, key);
    });
    setDrafts(nextDrafts);
    setExposed((prev) => {
      const validIds = new Set(nextDrafts.map((d) => d.id));
      return new Set([...prev].filter((id) => validIds.has(id)));
    });
    setMode("structured");
  }
  function formatModelsJson() {
    try {
      setText(JSON.stringify(JSON.parse(text), null, 2));
    } catch {}
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

  function removeDraft(key: string) {
    const removed = drafts.find((d) => d.key === key);
    setDrafts((prev) => prev.filter((d) => d.key !== key));
    setExpandedKeys((s) => {
      const ns = new Set(s);
      ns.delete(key);
      return ns;
    });
    if (removed) {
      setExposed((s) => {
        const ns = new Set(s);
        ns.delete(removed.id);
        return ns;
      });
    }
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
      const { models: ids, enrich } = await api.fetchModels(name);
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

  // 实时校验仅用于保存时阻断与行内高亮，不再全局黄条常驻（避免空行误导）
  const validationMsg = null as unknown as string | null; // 保留变量名以兼容后续引用，但置空

  async function saveLocal() {
    let models: ModelEntry[];
    if (mode === "raw") {
      if (!jsonValidation.ok || !jsonValidation.value) {
        const msg = jsonValidation.error ?? "Invalid JSON";
        setValidationError(msg);
        toast("err", msg);
        return;
      }
      models = (jsonValidation.value as unknown as ModelEntry[]).map((p) => {
        if (!p.contextWindow || typeof p.contextWindow !== "number") (p as any).contextWindow = 128000;
        if (!p.maxTokens || typeof p.maxTokens !== "number") (p as any).maxTokens = 16384;
        if (!p.input || (Array.isArray(p.input) && p.input.length === 0)) (p as any).input = ["text"];
        return p as ModelEntry;
      });
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
      models = drafts.map((d) => {
        const p = modelPreview(d) as unknown as ModelEntry;
        if (!p.contextWindow || typeof p.contextWindow !== "number") (p as any).contextWindow = 128000;
        if (!p.maxTokens || typeof p.maxTokens !== "number") (p as any).maxTokens = 16384;
        if (!p.input || (Array.isArray(p.input) && p.input.length === 0)) (p as any).input = ["text"];
        return p;
      });
    }
    const res = await api.updateModels(name, models);
    if ((res as any).enrich) {
      const e = (res as any).enrich;
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
              <span className="text-zinc-400">{t("Checked = exposed to pi as")}</span> <code>{name}/&lt;id&gt;</code>
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
            <JsonEditor value={text} onChange={setText} label="models json" className="h-64 sm:h-80" errorLine={modelsErrorLine} />
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

function ProfileCardMenu({
  name,
  onTest,
  onCopy,
  onDelete,
}: {
  name: string;
  onTest: () => void;
  onCopy: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const { t } = useI18n() as any;
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);
  return (
    <div className="relative">
      <Button
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t("Actions for {{name}}").replace("{{name}}", name)}
        onClick={() => setOpen((v) => !v)}
        className="px-2"
      >
        …
      </Button>
      {open && (
        <>
          <button className="fixed inset-0 z-[55]" aria-label={t("Close menu")} onClick={() => setOpen(false)} />
          <div role="menu" className="absolute right-0 z-[55] mt-1 w-36 rounded-lg border border-line bg-zinc-900 p-1 shadow-xl">
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-white/5" onClick={() => { setOpen(false); onTest(); }}>{t("Test")}</button>
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-white/5" onClick={() => { setOpen(false); onCopy(); }}>{t("Copy")}</button>
            <div className="my-1 h-px bg-white/10" />
            <button role="menuitem" className="w-full rounded px-3 py-1.5 text-left text-sm text-red-300 hover:bg-red-500/10" onClick={() => { setOpen(false); onDelete(); }}>{t("Delete")}</button>
          </div>
        </>
      )}
    </div>
  );
}


// ─── preset loader ────────────────────────────────────────

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

// ─── cc-switch import modal ───────────────────────────────

function CcsImportModal({
  onClose,
  onImported,
}: {
  onClose: () => void;
  onImported: () => Promise<void>;
}) {
  const run = useAction();
  const toast = useToast();
  const { t, lang } = useI18n() as any;
  const [providers, setProviders] = useState<CcsProvider[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [path, setPath] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const load = async (p?: string) => {
    setError(null);
    setProviders(null);
    try {
      const data = await api.ccsProviders(p || undefined);
      setProviders(data.providers);
      setSelected(new Set(data.providers.filter((x) => !x.exists).map((x) => x.id)));
    } catch (err) {
      setError((err as Error).message);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const doImport = async () => {
    const selections = [...selected].map((id) => ({ id }));
    if (selections.length === 0) return;
    const data = await api.importCcs(selections, path || undefined);
    if (data.imported > 0) {
      alert(t("Imported {{n}} provider(s) from cc-switch").replace("{{n}}", String(data.imported)));
      await onImported();
    } else {
      alert(t("Nothing imported (already exist or skipped)."));
    }
  };

  const apiLabel = (api: string) =>
    ({ "anthropic-messages": "anthropic", "openai-responses": "openai", "google-generative-ai": "gemini" })[api] ?? api;

  return (
    <Modal title={t("Import from cc-switch")} onClose={onClose} wide>
      <div className="space-y-3">
        {error && (
          <div>
            <p style={{ color: "#ff5555", fontSize: "0.9rem" }}>{error}</p>
            <div className="flex gap-2 mt-2">
              <Input
                placeholder={t("Path to cc-switch.db (optional)")}
                value={path}
                onChange={(e) => setPath(e.target.value)}
              />
              <Button onClick={() => void load(path)}>{t("Retry")}</Button>
            </div>
          </div>
        )}

        {providers === null && !error && <p style={{ color: "#999" }}>{t("Loading…")}</p>}

        {providers && providers.length === 0 && (
          <p style={{ color: "#999" }}>{t("No importable providers found in cc-switch.")}</p>
        )}

        {providers && providers.length > 0 && (
          <div className="space-y-2 max-h-80 overflow-auto">
            {providers.map((p) => (
              <label
                key={p.id}
                className="flex items-start gap-2 p-2 rounded cursor-pointer"
                style={{ background: selected.has(p.id) ? "#f0f7ff" : "#fafafa" }}
              >
                <input
                  type="checkbox"
                  checked={selected.has(p.id)}
                  onChange={() => toggle(p.id)}
                />
                <span className="text-sm">
                  <strong>{p.name}</strong>{" "}
                  <Badge>{apiLabel(p.api)}</Badge>{" "}
                  {p.exists && <Badge>{t("exists")}</Badge>}
                  <br />
                  <span style={{ color: "#999" }}>{p.baseUrl}</span>
                  <br />
                  <span style={{ color: "#666" }}>{p.models.join(", ") || "-"}</span>
                </span>
              </label>
            ))}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <Button onClick={onClose}>{t("Cancel")}</Button>
          <Button variant="primary" disabled={!providers || selected.size === 0} onClick={() => void doImport()}>
            {t("Import selected")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

import { useEffect, useMemo, useState } from "react";
import type { AppState, ModelEntry, PresetInfo, ProviderProfile, ResponsesMode, Upstream } from "../types";
import { hasUpstreams, resolvedUpstreams } from "../types";
import { effectiveResponsesMode, responsesModeError } from "../lib/responsesMode";
import { defaultProtocolApiId, protocolApiIds, protocolCapabilities } from "../lib/protocolCapabilities";
import { draftFromEntry, entryFromDraft, modelPreview, newModelDraft, validateModelsJson, validateProfileJson, type ModelDraft } from "../lib/piModel";
import { channelNames, exposedForChannel, materializeMainChannel, modelsForChannel, parseModelsDraft, useProfileDraft } from "../hooks/useProfileDraft";
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
import { useDebounce } from "../hooks/useDebounce";
import { mergePreviewHeaders } from "../lib/previewHeaders";
import { mutateAfterProfilePut } from "../store/swr";
// Api list and labels come from the backend capability set (GET /api/state),
// never from a local copy; the fallback keeps rendering before the first fetch.
function apiTypeOptions(): ReadonlyArray<{ value: string; label: string }> {
  return protocolCapabilities().map((c) => ({ value: c.id, label: c.label }));
}
const SPOOFS = [
  { value: "", label: "none" },
  { value: "claude-code", label: "claude-code" },
  { value: "codex", label: "codex" },
  { value: "gemini", label: "gemini" },
];

type UpstreamForm = Omit<Upstream, "api" | "responsesMode" | "weight" | "name" | "headers" | "models" | "exposedModels"> & {
  key: string;
  api: string;
  responsesMode: ResponsesMode;
  weight: string;
  name: string;
  headers: Record<string, string>;
  models: ModelEntry[];
  exposedModels: string[];
};

function emptyProfile(): ProviderProfile {
  return {
    api: defaultProtocolApiId(),
    responsesMode: "auto",
    baseUrl: "",
    apiKey: "",
    proxy: false,
    headers: {},
    compat: {},
    upstreams: [],
  };
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

  const entries = Object.entries(state.profiles).sort(([a], [b]) => a.localeCompare(b));

  return (
    <div>
      <SectionTitle hint={`${entries.length} ${t("profile(s)")}`}>{t("Profiles")}</SectionTitle>

      <div className="mb-3 flex gap-2">
        <Button variant="primary" onClick={() => setEditing({ name: null })}>
          {t("+ Add profile")}
        </Button>
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
          const exposed = (p.upstreams ?? []).reduce((n, u) => n + (u.exposedModels?.length ?? 0), 0);
          const modelCount = (p.upstreams ?? []).reduce((n, u) => n + (u.models?.length ?? 0), 0);
          const upstreamsList = hasUpstreams(p) ? resolvedUpstreams(p) : [];
          const mainUrl = (upstreamsList[0]?.baseUrl || p.baseUrl) || t("no base url");
          return (
            <Card
              key={name}
              variant={isCurrent ? "active" : "default"}
              className="flex flex-col gap-3 transition-all"
            >
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="truncate font-mono text-[14px] font-semibold text-zinc-100">{name}</span>
                    {isCurrent && (
                      <Badge tone="amber" dot mono>
                        {t("current")}
                      </Badge>
                    )}
                    {p.proxy && (
                      <Badge tone="indigo" mono>
                        {t("proxy")}
                      </Badge>
                    )}
                    <Badge mono>{p.api || "?"}</Badge>
                    <Badge tone="amber" mono>
                      {t("Responses")}: {effectiveResponsesMode(p)}
                    </Badge>
                    {exposed > 0 && (
                      <Badge tone="green" dot mono>
                        {exposed} {t("exposed")}
                      </Badge>
                    )}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-2 font-mono text-xs text-zinc-400">
                    <span className="truncate max-w-md text-zinc-300" title={mainUrl}>
                      {mainUrl}
                    </span>
                    <span className="text-zinc-600">·</span>
                    <span>{modelCount} {t("models")}</span>
                    {hasUpstreams(p) && (
                      <>
                        <span className="text-zinc-600">·</span>
                        <span className="text-indigo-400">{upstreamsList.length} upstream(s)</span>
                      </>
                    )}
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-1.5 lg:shrink-0 lg:justify-end">
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
  const draft = useProfileDraft(existing ?? emptyProfile());
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
  const setUpstreams = (next: UpstreamForm[] | ((prev: UpstreamForm[]) => UpstreamForm[])) => {
    const resolved = typeof next === "function" ? next(upstreams) : next;
    setField("upstreams", resolved.map(({ key: _key, weight, name: upstreamName, ...upstream }) => ({
      ...upstream,
      name: upstreamName.trim() || undefined,
      weight: weight.trim() ? Number(weight) : undefined,
      headers: Object.keys(upstream.headers ?? {}).length ? upstream.headers : undefined,
    })));
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

  function build(): ProviderProfile {
    const next = JSON.parse(JSON.stringify(draft.state.value)) as ProviderProfile & {
      models?: ModelEntry[];
      exposedModels?: string[];
    };
    if (!next.upstreams || next.upstreams.length === 0) {
      next.upstreams = [{
        name: "main",
        api: next.api,
        responsesMode: next.responsesMode ?? "auto",
        baseUrl: next.baseUrl.trim(),
        apiKey: next.apiKey.trim(),
        headers: next.headers,
        models: next.models ?? [],
        exposedModels: next.exposedModels ?? [],
      }];
      delete next.models;
      delete next.exposedModels;
    }
    next.updatedAt = new Date().toISOString();
    return next;
  }

  async function saveLocal() {
    const trimmed = name.trim();
    if (!trimmed) throw new Error(t("name required"));
    if (mode === "raw" && (!jsonValidation.ok || !jsonValidation.value)) {
      throw new Error(jsonValidation.error ?? "Invalid JSON");
    }
    const modeError = responsesModeError(value.api, value.responsesMode ?? "auto");
    if (modeError) throw new Error(t(modeError));
    const profile = build();
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
                {apiTypeOptions().map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
                {!protocolApiIds().includes(apiType) && apiType && (
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
                  <div data-testid="preview-headers" className="mt-2 rounded border border-white/10 bg-zinc-900/30 p-2 text-xs">
                    <div className="text-zinc-500">Preview headers (debounced 300ms, X-Custom merged, Upstream &gt; Profile, not auto-saved):</div>
                    <pre className="mt-1 whitespace-pre-wrap break-words text-zinc-300">{JSON.stringify(previewHeaders, null, 2)}</pre>
                    <div className="mt-1 text-zinc-500">Upstream aggregated: {JSON.stringify(previewUpstreamAggregated)}</div>
                    <div className="text-zinc-500">UserAgent preview: {debouncedSpoof || "(none)"}</div>
                  </div>
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
                        const first: any = { key: `us-${Date.now()}`, baseUrl: baseUrl.trim(), apiKey: apiKey.trim(), api: apiType, responsesMode, weight: "", name: "main", headers: { ...headers }, models: [], exposedModels: [] };
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
                        onClick={() => setUpstreams((prev) => [...prev, { key: `us-${Date.now()}-${prev.length}`, baseUrl: "", apiKey: "", api: apiType, responsesMode, weight: "", name: "", headers: {}, models: [], exposedModels: [] }])}
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
                  <div className="text-xs text-zinc-500">{t("每个 channel 都需要名称和 API 类型；模型池在 Models 中按 channel 管理。")}</div>
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
                        <Field label={t("API type")}>
                          <Select value={u.api} onChange={(e) => setUpstreams((prev) => prev.map((x) => x.key === u.key ? { ...x, api: e.target.value } : x))}>
                            {apiTypeOptions().map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
                          </Select>
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


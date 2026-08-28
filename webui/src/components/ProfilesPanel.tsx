import { useEffect, useMemo, useState } from "react";
import type { AppState, CcsProvider, ModelEntry, PresetInfo, ProviderProfile, ResponsesMode } from "../types";
import { effectiveResponsesMode, responsesModeError } from "../lib/responsesMode";
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
import { GatewayPreviewModal } from "./GatewayPreviewModal";
const API_TYPES = [
  "openai-completions",
  "openai-responses",
  "anthropic-messages",
  "google-generative-ai",
];
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
            <Card key={name} className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="truncate font-medium text-zinc-100">{name}</span>
                  {isCurrent && <Badge tone="indigo">{t("current")}</Badge>}
                  {p.proxy && <Badge tone="amber">{t("proxy")}</Badge>}
                  <Badge>{p.api || "?"}</Badge>
                  <Badge tone="indigo">{t("Responses")}: {effectiveResponsesMode(p)}</Badge>
                  {exposed > 0 && <Badge tone="green">{exposed} {t("exposed")}</Badge>}
                </div>
                <div className="mt-0.5 truncate text-xs text-zinc-500">
                  {p.baseUrl || t("no base url")} · {p.models?.length ?? 0} {t("models")}
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
                <Button
                  onClick={() =>
                    run(
                      async () => {
                        const r = await api.testProfile(name);
                        if (!r.success) throw new Error(r.message);
                        return r;
                      },
                      t("Test OK"),
                    )
                  }
                >
                  {t("Test")}
                </Button>
                <Button
                  onClick={() => {
                    const to = prompt(
                      `${t("Duplicate profile '{{name}}' as:").replace("{{name}}", name)}`,
                      `${name}-copy`,
                    );
                    if (to) run(() => api.duplicateProfile(name, to), t("Duplicated"), refresh);
                  }}
                >
                  {t("Copy")}
                </Button>
                <Button
                  variant="danger"
                  onClick={() => {
                    if (confirm(t("Delete profile '{{name}}'?").replace("{{name}}", name)))
                      run(() => api.deleteProfile(name), t("Deleted"), refresh);
                  }}
                >
                  {t("Delete")}
                </Button>
              </div>
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
  const [modelIds, setModelIds] = useState(
    (existing?.models ?? []).map((m) => m.id).join("\n"),
  );
  const [gatewayPreview, setGatewayPreview] = useState<{ current: unknown; proposed: unknown; conflicts: string[] } | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

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
    return {
      ...(existing ?? {}),
      api: apiType,
      responsesMode,
      baseUrl: baseUrl.trim(),
      apiKey: apiKey.trim(),
      models,
      proxy,
      exposedModels,
      preset: preset || undefined,
      modelsDevProvider: modelsDevProvider.trim() || undefined,
      userAgent: spoof || undefined,
      updatedAt: new Date().toISOString(),
    } as ProviderProfile;
  }

  async function saveWithPreview() {
    const trimmed = name.trim();
    if (!trimmed) throw new Error(t("name required"));
    const modeError = responsesModeError(apiType, responsesMode);
    if (modeError) throw new Error(t(modeError));
    setPreviewLoading(true);
    try {
      const preview = await api.previewGateway();
      setGatewayPreview(preview as any);
    } finally {
      setPreviewLoading(false);
    }
  }

  async function handlePreviewConfirm(edited: unknown) {
    const trimmed = name.trim();
    const profile = build();
    // Apply edited gateway first so its hand-written extra is preserved when the profile save re-syncs
    if (gatewayPreview && JSON.stringify(edited) !== JSON.stringify((gatewayPreview as any).proposed)) {
      await api.applyGateway(edited);
    }
    if (original) {
      await api.updateProfile(trimmed, profile, original !== trimmed ? original : undefined);
    } else {
      await api.addProfile(trimmed, profile);
    }
    setGatewayPreview(null);
    await onSaved();
  }

  return (
    <>
      <Modal title={original ? `${t("Edit")} ${original}` : t("Add profile")} onClose={onClose} wide>
        <div className="grid gap-x-4 sm:grid-cols-2">
          <Field label={t("Name")}>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="my-provider" />
          </Field>
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
              {API_TYPES.map((a) => (
                <option key={a} value={a}>
                  {a}
                </option>
              ))}
            </Select>
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

        <div className="mt-2 flex justify-end gap-2">
          <Button onClick={onClose}>{t("Cancel")}</Button>
          <Button variant="primary" disabled={previewLoading} onClick={() => run(saveWithPreview, undefined)}>
            {previewLoading ? t("Loading…") : t("Save")}
          </Button>
        </div>
      </Modal>
      {gatewayPreview && (
        <GatewayPreviewModal
          current={gatewayPreview.current as any}
          proposed={gatewayPreview.proposed as any}
          conflicts={gatewayPreview.conflicts}
          onClose={() => setGatewayPreview(null)}
          onConfirm={(edited) => run(() => handlePreviewConfirm(edited), t("Saved"), undefined)}
        />
      )}
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
  const [models, setModels] = useState<ModelEntry[]>(profile.models ?? []);
  const [exposed, setExposed] = useState<Set<string>>(
    new Set(profile.exposedModels ?? []),
  );
  const [newId, setNewId] = useState("");
  const [fetching, setFetching] = useState(false);
  const [gatewayPreview, setGatewayPreview] = useState<{ current: unknown; proposed: unknown; conflicts: string[] } | null>(null);

  function toggle(id: string) {
    setExposed((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function addId(id: string) {
    const trimmed = id.trim();
    if (!trimmed || models.some((m) => m.id === trimmed)) return;
    setModels((m) => [...m, defaultModel(trimmed)]);
  }

  async function fetchFromProvider() {
    setFetching(true);
    try {
      const { models: ids, enrich } = await api.fetchModels(name);
      setModels((prev) => {
        const have = new Set(prev.map((m) => m.id));
        const added = ids.filter((id) => !have.has(id)).map(defaultModel);
        return [...prev, ...added];
      });
      // 可观测性：上游模型列表 + 模型目录 enrich 统计（术语：模型目录/模型元数据 vs 上游模型列表）
      if (enrich) {
        const isZh = (lang as string) === "zh";
        const base = t("Fetch from provider");
        const enrichMsg = enrich.failed > 0
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

  async function saveWithPreview() {
    const preview = await api.previewGateway();
    setGatewayPreview(preview as any);
  }

  async function handlePreviewConfirm(edited: unknown) {
    if (gatewayPreview && JSON.stringify(edited) !== JSON.stringify((gatewayPreview as any).proposed)) {
      await api.applyGateway(edited);
    }
    const res = await api.updateModels(name, models);
    // 可观测性：模型目录 enrich 结果合并到保存提示
    if (res.enrich) {
      const isZh = (lang as string) === "zh";
      const e = res.enrich;
      const enrichMsg = e.failed > 0
        ? isZh
          ? `模型元数据 enrich 失败 ${e.failed} 条` + (e.warning ? ` · ${e.warning}` : "")
          : `model metadata enrich failed ${e.failed}` + (e.warning ? ` · ${e.warning}` : "")
        : isZh
          ? `已 enrich ${e.enriched} 条，跳过 ${e.skipped} 条（模型目录未覆盖）` + (e.warning ? ` · ${e.warning}` : "")
          : `enriched ${e.enriched}, skipped ${e.skipped} (not in catalog)` + (e.warning ? ` · ${e.warning}` : "");
      toast("ok", enrichMsg);
    }
    await api.expose(name, [...exposed].filter((id) => models.some((m) => m.id === id)));
    setGatewayPreview(null);
    await onSaved();
  }

  return (
    <>
      <Modal title={`${t("Models")} · ${name}`} onClose={onClose} wide>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Input
            value={newId}
            onChange={(e) => setNewId(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                addId(newId);
                setNewId("");
              }
            }}
            placeholder={t("add model id + Enter")}
            className="max-w-xs"
          />
          <Button
            onClick={() => void fetchFromProvider()}
            disabled={fetching}
          >
            {fetching ? t("Fetching…") : t("Fetch from provider")}
          </Button>
          <span className="text-xs text-zinc-500">
            {t("Checked = exposed to pi as")} <code>{name}/&lt;id&gt;</code>
          </span>
        </div>

        <div className="max-h-80 space-y-1 overflow-y-auto rounded-lg border border-white/10 p-2">
          {models.length === 0 && (
            <div className="p-3 text-sm text-zinc-500">
              {t("No models. Add ids above or fetch from the provider.")}
            </div>
          )}
          {models.map((m) => (
            <div
              key={m.id}
              className={cx(
                "flex items-center justify-between rounded-md px-2 py-1.5 text-sm",
                exposed.has(m.id) ? "bg-emerald-500/10" : "hover:bg-white/5",
              )}
            >
              <label className="flex min-w-0 items-center gap-2">
                <input
                  type="checkbox"
                  checked={exposed.has(m.id)}
                  onChange={() => toggle(m.id)}
                />
                <span className="truncate text-zinc-200">{m.id}</span>
              </label>
              <button
                className="text-xs text-zinc-500 hover:text-red-300"
                onClick={() => {
                  setModels((prev) => prev.filter((x) => x.id !== m.id));
                  setExposed((prev) => {
                    const n = new Set(prev);
                    n.delete(m.id);
                    return n;
                  });
                }}
              >
                {t("remove")}
              </button>
            </div>
          ))}
        </div>

        <div className="mt-4 flex items-center justify-between">
          <div className="flex gap-2 text-xs">
            <button
              className="text-zinc-400 hover:text-zinc-200"
              onClick={() => setExposed(new Set(models.map((m) => m.id)))}
            >
              {t("expose all")}
            </button>
            <button
              className="text-zinc-400 hover:text-zinc-200"
              onClick={() => setExposed(new Set())}
            >
              {t("expose none")}
            </button>
          </div>
          <div className="flex gap-2">
            <Button onClick={onClose}>{t("Cancel")}</Button>
            <Button variant="primary" onClick={() => run(saveWithPreview, undefined)}>
              {t("Save")}
            </Button>
          </div>
        </div>
      </Modal>
      {gatewayPreview && (
        <GatewayPreviewModal
          current={gatewayPreview.current as any}
          proposed={gatewayPreview.proposed as any}
          conflicts={gatewayPreview.conflicts}
          onClose={() => setGatewayPreview(null)}
          onConfirm={(edited) => run(() => handlePreviewConfirm(edited), t("Models saved"), undefined)}
        />
      )}
    </>
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

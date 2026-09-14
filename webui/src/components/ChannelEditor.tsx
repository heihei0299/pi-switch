import type { ModelEntry, ProtocolApiCapability, ResponsesMode, Upstream } from "../types";
import { protocolCapabilities } from "../lib/protocolCapabilities";
import { RequestHeadersEditor } from "./RequestHeadersEditor";
import { Button, Field, Input, Select } from "./ui";
import { useI18n } from "../i18n";

// Api list and labels come from the backend capability set (GET /api/state),
// never from a local copy; the fallback keeps rendering before the first fetch.
// An api the proxy cannot serve yet stays visible but not selectable: the write
// doors reject it (system-contract §2.2), so offering it would only produce a
// failed save. It has to stay visible rather than be filtered out, because a
// profile already using it must still render its own value.
export function apiTypeOptions(caps: readonly ProtocolApiCapability[]): ReadonlyArray<{ value: string; label: string; disabled: boolean }> {
  return protocolCapabilities(caps).map((c) => ({ value: c.id, label: c.label, disabled: !c.canProxy }));
}

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

export type { UpstreamForm };
export type UpstreamChange = UpstreamForm[] | ((previous: UpstreamForm[]) => UpstreamForm[]);

export interface ChannelEditorProps {
  apiType: string;
  responsesMode: ResponsesMode;
  caps: readonly ProtocolApiCapability[];
  baseUrl: string;
  apiKey: string;
  headers: Record<string, string>;
  upstreams: UpstreamForm[];
  debouncedSpoof: string;
  previewHeaders: Record<string, string>;
  previewUpstreamAggregated: Record<string, string>;
  onBaseUrlChange: (value: string) => void;
  onApiKeyChange: (value: string) => void;
  onHeadersChange: (value: Record<string, string>) => void;
  onUpstreamsChange: (value: UpstreamChange) => void;
}

export function ChannelEditor({
  apiType,
  responsesMode,
  caps,
  baseUrl,
  apiKey,
  headers,
  upstreams,
  debouncedSpoof,
  previewHeaders,
  previewUpstreamAggregated,
  onBaseUrlChange,
  onApiKeyChange,
  onHeadersChange,
  onUpstreamsChange,
}: ChannelEditorProps) {
  const { t } = useI18n() as any;

  return (
    <>
      {upstreams.length === 0 ? (
        <>
          <div className="sm:col-span-2">
            <Field label={t("Base URL")}>
              <Input
                value={baseUrl}
                onChange={(e) => onBaseUrlChange(e.target.value)}
                placeholder="https://api.example.com/v1"
              />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label={t("API key (supports $ENV_VAR)")}>
              <Input value={apiKey} onChange={(e) => onApiKeyChange(e.target.value)} placeholder="sk-…" />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <RequestHeadersEditor headers={headers} onHeadersChange={onHeadersChange} />
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
                  const first: UpstreamForm = { key: `us-${Date.now()}`, baseUrl: baseUrl.trim(), apiKey: apiKey.trim(), api: apiType, responsesMode, weight: "", name: "main", headers: { ...headers }, models: [], exposedModels: [] };
                  onUpstreamsChange([first]);
                }}
              >
                {t("Manage upstreams")}
              </Button>
            )}
            {upstreams.length > 0 && (
              <>
                <Button
                  type="button"
                  onClick={() => onUpstreamsChange((prev) => [...prev, { key: `us-${Date.now()}-${prev.length}`, baseUrl: "", apiKey: "", api: apiType, responsesMode, weight: "", name: "", headers: {}, models: [], exposedModels: [] }])}
                >
                  + {t("Add upstream")}
                </Button>
                <Button
                  type="button"
                  onClick={() => {
                    if (upstreams.length > 0) {
                      onBaseUrlChange(upstreams[0].baseUrl);
                      onApiKeyChange(upstreams[0].apiKey);
                      onHeadersChange(upstreams[0].headers ?? {});
                    }
                    onUpstreamsChange([]);
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
                  <Button type="button" onClick={() => onUpstreamsChange((prev) => prev.filter((x) => x.key !== u.key))} className="h-7 text-xs">{t("Remove")}</Button>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label={t("Base URL")}>
                    <Input value={u.baseUrl} onChange={(e) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, baseUrl: e.target.value } : x))} placeholder="https://api.example.com/v1" />
                  </Field>
                  <Field label={t("API key")}>
                    <Input value={u.apiKey} onChange={(e) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, apiKey: e.target.value } : x))} placeholder="sk-…" />
                  </Field>
                  <Field label={t("Weight")}>
                    <Input value={u.weight} onChange={(e) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, weight: e.target.value } : x))} placeholder="1" />
                  </Field>
                  <Field label={t("Name")}>
                    <Input value={u.name} onChange={(e) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, name: e.target.value } : x))} placeholder="upstream-a" />
                  </Field>
                  <Field label={t("API type")}>
                    <Select value={u.api} onChange={(e) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, api: e.target.value } : x))}>
                      {apiTypeOptions(caps).map((o) => <option key={o.value} value={o.value} disabled={o.disabled}>{o.label}</option>)}
                    </Select>
                  </Field>
                </div>
                <div className="mt-2">
                  <RequestHeadersEditor headers={u.headers} onHeadersChange={(next) => onUpstreamsChange((prev) => prev.map((x) => x.key === u.key ? { ...x, headers: next } : x))} />
                </div>
              </div>
            ))}
            {upstreams.length === 0 && <div className="text-xs text-zinc-500">{t("No upstreams yet.")}</div>}
          </div>
        )}
      </div>
    </>
  );
}

import type {
  AppState,
  CcsImportResult,
  CcsProvider,
  ConversationRequestsPage,
  ConversationsPage,
  DaemonResult,
  DoctorCheck,
  EnrichStats,
  ModelEntry,
  PackageEntry,
  PackageImportResult,
  PresetInfo,
  ProfileDetail,
  ProviderProfile,
  TestResult,
  UsageStats,
  ValidationIssue,
  GatewayPreview,
  GatewaySelection,
} from "./types";
import type { ConversationRange, StatsRange } from "./lib/statsWindow";
import type { NormalizedCredits } from "./lib/credits";
import type { BuildInfo, GatewayHealth } from "./apiSchema";
import {
  ContractError,
  decodeAppState,
  decodeBuildInfo,
  decodeCcsImport,
  decodeCcsProviders,
  decodeConversationRequestsPage,
  decodeConversationsPage,
  decodeCredits,
  decodeDaemonResult,
  decodeDoctorChecks,
  decodeFetchModels,
  decodeGatewayEnvelope,
  decodeGatewayHealth,
  decodeGatewayPreview,
  decodeGatewayStart,
  decodeImportPackages,
  decodeMessageList,
  decodeOk,
  decodeOkResult,
  decodePackage,
  decodePackages,
  decodePresetProfile,
  decodePresets,
  decodeProfileDetail,
  decodeProviderProfile,
  decodeStringArray,
  decodeTestResult,
  decodeUpdateModels,
  decodeUsageStats,
  decodeValidationIssues,
  decodeWebUIInfo,
  decodeExportConfig,
  decodeImportConfig,
  decodeRestoreConfig,
} from "./apiSchema";
import type { Decoder } from "./apiSchema";

// Single point of coupling to the backend. Every call maps to one REST route in
// internal/server/server.go, which in turn delegates to the shared Go core.
async function req<T>(method: string, path: string, body: unknown, decode: Decoder<T>): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new ContractError("$", "valid JSON response");
    }
  }
  if (!res.ok) {
    const error = data && typeof data === "object" && "error" in data
      ? (data as { error?: unknown }).error
      : undefined;
    throw new Error(typeof error === "string" ? error : res.statusText || "request failed");
  }
  return decode(data);
}

const enc = encodeURIComponent;

export const api = {
  // reads
  getState: () => req<AppState>("GET", "/state", undefined, decodeAppState),
  getPresets: () => req<PresetInfo[]>("GET", "/presets", undefined, decodePresets),
  getPreset: (id: string) => req<ProviderProfile & { name?: string }>("GET", `/presets/${enc(id)}`, undefined, decodePresetProfile),
  getProfile: (name: string) => req<ProfileDetail>("GET", `/profiles/${enc(name)}`, undefined, decodeProfileDetail),
  doctor: () => req<DoctorCheck[]>("GET", "/doctor", undefined, decodeDoctorChecks),
  validate: () => req<ValidationIssue[]>("GET", "/config/validate", undefined, decodeValidationIssues),
  backups: () => req<string[]>("GET", "/backups", undefined, (value) => decodeStringArray(value, "backups")),
  stats: (range: StatsRange, from: number, to: number, page = 0, limit = 50) =>
    req<UsageStats>(
      "GET",
      `/stats?range=${range}&from=${from}&to=${to}&page=${page}&limit=${limit}`,
      undefined,
      decodeUsageStats,
    ),
  statsConversations: (
    range: ConversationRange,
    from: number | null,
    to: number | null,
    page = 0,
    limit = 50,
  ) => {
    // "all" means full history: omit the window params so the backend keeps
    // the null-window (no params) behaviour.
    const params =
      range === "all"
        ? `page=${page}&limit=${limit}`
        : `range=${range}&from=${from}&to=${to}&page=${page}&limit=${limit}`;
    return req<ConversationsPage>("GET", `/stats/conversations?${params}`, undefined, decodeConversationsPage);
  },
  conversationRequests: (id: string, page = 0, limit = 50) =>
    req<ConversationRequestsPage>(
      "GET",
      `/stats/conversations/${enc(id)}/requests?page=${page}&limit=${limit}`,
      undefined,
      decodeConversationRequestsPage,
    ),
  proxyStatus: () => req<DaemonResult>("GET", "/proxy/status", undefined, decodeDaemonResult),
  buildInfo: () => req<BuildInfo>("GET", "/buildInfo", undefined, decodeBuildInfo),
  webuiInfo: () => req<{ authRequired: boolean }>("GET", "/webui/info", undefined, decodeWebUIInfo),

  // package management
  getPackages: () => req<{ packages: PackageEntry[] }>("GET", "/packages", undefined, decodePackages),
  getPackage: (id: string) => req<PackageEntry>("GET", `/packages/${enc(id)}`, undefined, decodePackage),
  addPackage: (spec: string) =>
    req("POST", "/packages", { spec, enabled: true }, decodeOk),
  importPackages: () =>
    req<PackageImportResult>("POST", "/packages/import", {}, decodeImportPackages),
  togglePackage: (id: string) => req("POST", `/packages/${enc(id)}/toggle`, undefined, decodeOk),
  deletePackage: (id: string) => req("DELETE", `/packages/${enc(id)}`, undefined, decodeOk),

  // cc-switch import
  ccsProviders: (path?: string) =>
    req<{ providers: CcsProvider[] }>("GET", `/ccswitch/providers${path ? `?path=${enc(path)}` : ""}`, undefined, decodeCcsProviders),
  importCcs: (selections: { id: string; force?: boolean }[], path?: string) =>
    req<{ ok: boolean; imported: number; results: CcsImportResult[] }>(
      "POST",
      "/ccswitch/import",
      { selections, path },
      decodeCcsImport,
    ),

  // profile mutations
  init: () => req<{ messages: string[] }>("POST", "/init", undefined, decodeMessageList),
  addProfile: (name: string, profile: ProviderProfile) =>
    req("POST", "/profiles", { name, profile }, decodeOk),
  updateProfile: (name: string, profile: ProviderProfile, renameFrom?: string) =>
    req("PUT", `/profiles/${enc(name)}`, { profile, renameFrom }, decodeOk),
  deleteProfile: (name: string) => req("DELETE", `/profiles/${enc(name)}`, undefined, decodeOk),
  duplicateProfile: (name: string, asName: string) =>
    req("POST", `/profiles/${enc(name)}/duplicate`, { as: asName }, decodeOk),
  testProfile: (name: string) =>
    req<TestResult>("POST", `/profiles/${enc(name)}/test`, undefined, decodeTestResult),
  fetchModels: (name: string, channel?: string) =>
    req<{ models: string[]; enrich?: EnrichStats }>(
      "POST",
      `/profiles/${enc(name)}/fetch-models${channel ? `?channel=${enc(channel)}` : ""}`,
      undefined,
      decodeFetchModels,
    ),
  updateModels: (name: string, models: ModelEntry[], channel?: string) =>
    req<{ ok: boolean; backup?: string | null; enrich?: EnrichStats }>(
      "PUT",
      `/profiles/${enc(name)}/models`,
      channel ? { models, channel } : { models },
      decodeUpdateModels,
    ),
  expose: (name: string, modelIds: string[], channel?: string) =>
    req("PUT", `/profiles/${enc(name)}/expose${channel ? `?channel=${enc(channel)}` : ""}`, { modelIds }, decodeOk),
  setSpoof: (name: string, spoof: string | null) =>
    req("PUT", `/profiles/${enc(name)}/spoof`, { spoof }, decodeOk),
  getCredits: (name: string) =>
    req<NormalizedCredits>("GET", `/profiles/${enc(name)}/credits`, undefined, decodeCredits),

  // proxy + settings + config
  proxyStart: (host?: string, port?: number) =>
    req<DaemonResult>("POST", "/proxy/start", { host, port }, decodeDaemonResult),
  proxyStop: () => req<DaemonResult>("POST", "/proxy/stop", undefined, decodeDaemonResult),
  updateSettings: (settings: AppState["settings"]) => req("PUT", "/settings", settings, decodeOk),
  getGateway: () => req<{ gateway: unknown }>("GET", "/models/gateway", undefined, decodeGatewayEnvelope),
  previewGateway: (input?: { selected?: GatewaySelection[]; draft?: unknown }) =>
    input === undefined
      ? req<GatewayPreview>("GET", "/models/gateway/preview", undefined, decodeGatewayPreview)
      : req<GatewayPreview>("POST", "/models/gateway/preview", input, decodeGatewayPreview),
  applyGateway: (gateway: unknown) => req<{ ok: boolean }>("PUT", "/models/gateway", gateway, decodeOkResult),
  getGatewayHealth: () => req<GatewayHealth>("GET", "/gateway/health", undefined, decodeGatewayHealth),
  startGateway: () => req<{ running: boolean; mode: string }>("POST", "/gateway/start", undefined, decodeGatewayStart),
  exportConfig: (passphrase: string) =>
    req<{ path: string }>("POST", "/config/export", { passphrase }, decodeExportConfig),
  importConfig: (filePath: string, passphrase: string) =>
    req<{ message: string }>("POST", "/config/import", { filePath, passphrase }, decodeImportConfig),
  restoreConfig: (backupPath: string) =>
    req<{ backup: string }>("POST", "/config/restore", { backupPath }, decodeRestoreConfig),
};

export function logsExportUrl(format: "json" | "csv"): string {
  return `/api/logs/export?format=${format}`;
}

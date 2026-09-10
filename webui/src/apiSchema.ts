import type {
  AppState,
  CcsImportResult,
  CcsProvider,
  ConversationRequestsPage,
  ConversationsPage,
  DaemonResult,
  DoctorCheck,
  EnrichStats,
  GatewayDiff,
  GatewayPreview,
  ModelCost,
  ModelEntry,
  ModelStats,
  PackageEntry,
  PresetInfo,
  PreviewEnrich,
  PreviewGroup,
  ProfileDetail,
  ProviderProfile,
  ProviderStats,
  RecentRequest,
  Settings,
  TestResult,
  TokenTotals,
  Upstream,
  UsageStats,
  ValidationIssue,
} from "./types";
import type { GoUsage, NormalizedCredits, UsageWindow } from "./lib/credits";

type JsonObject = Record<string, unknown>;
type PathDecoder<T> = (value: unknown, path: string) => T;

export type Decoder<T> = (value: unknown) => T;

export class ContractError extends Error {
  readonly path: string;
  readonly expected: string;

  constructor(path: string, expected: string) {
    super(`${path}: ${expected}`);
    this.name = "ContractError";
    this.path = path;
    this.expected = expected;
    Object.setPrototypeOf(this, ContractError.prototype);
  }
}

function fail(path: string, expected: string): never {
  throw new ContractError(path, expected);
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function object(value: unknown, path: string): JsonObject {
  if (!isObject(value)) fail(path, "object");
  return value;
}

function has(value: JsonObject, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function withAliases(value: JsonObject, aliases: Record<string, string[]>): JsonObject {
  const out = { ...value };
  for (const [canonical, candidates] of Object.entries(aliases)) {
    if (has(out, canonical)) continue;
    for (const candidate of candidates) {
      if (has(out, candidate)) {
        out[canonical] = out[candidate];
        break;
      }
    }
  }
  return out;
}

function requiredString(value: JsonObject, key: string, path: string): string {
  const fieldPath = `${path}.${key}`;
  if (!has(value, key) || typeof value[key] !== "string") {
    fail(fieldPath, "required string");
  }
  return value[key] as string;
}

function defaultString(value: JsonObject, key: string, path: string, fallback: string): string {
  if (!has(value, key)) return fallback;
  if (typeof value[key] !== "string") fail(`${path}.${key}`, "string");
  return value[key] as string;
}

function optionalString(value: JsonObject, key: string, path: string): string | undefined {
  if (!has(value, key)) return undefined;
  if (typeof value[key] !== "string") fail(`${path}.${key}`, "string");
  return value[key] as string;
}

function nullableString(value: JsonObject, key: string, path: string): string | null | undefined {
  if (!has(value, key)) return undefined;
  if (value[key] === null) return null;
  if (typeof value[key] !== "string") fail(`${path}.${key}`, "string or null");
  return value[key] as string;
}

function requiredBoolean(value: JsonObject, key: string, path: string): boolean {
  const fieldPath = `${path}.${key}`;
  if (!has(value, key) || typeof value[key] !== "boolean") {
    fail(fieldPath, "required boolean");
  }
  return value[key] as boolean;
}

function defaultBoolean(value: JsonObject, key: string, path: string, fallback: boolean): boolean {
  if (!has(value, key)) return fallback;
  if (typeof value[key] !== "boolean") fail(`${path}.${key}`, "boolean");
  return value[key] as boolean;
}

function optionalBoolean(value: JsonObject, key: string, path: string): boolean | undefined {
  if (!has(value, key)) return undefined;
  if (typeof value[key] !== "boolean") fail(`${path}.${key}`, "boolean");
  return value[key] as boolean;
}

function requiredNumber(value: JsonObject, key: string, path: string): number {
  const fieldPath = `${path}.${key}`;
  if (!has(value, key) || typeof value[key] !== "number" || !Number.isFinite(value[key])) {
    fail(fieldPath, "required number");
  }
  return value[key] as number;
}

function defaultNumber(value: JsonObject, key: string, path: string, fallback: number): number {
  if (!has(value, key)) return fallback;
  if (typeof value[key] !== "number" || !Number.isFinite(value[key])) {
    fail(`${path}.${key}`, "number");
  }
  return value[key] as number;
}

function optionalNumber(value: JsonObject, key: string, path: string): number | undefined {
  if (!has(value, key)) return undefined;
  if (typeof value[key] !== "number" || !Number.isFinite(value[key])) {
    fail(`${path}.${key}`, "number");
  }
  return value[key] as number;
}

function nullableNumber(value: JsonObject, key: string, path: string): number | null | undefined {
  if (!has(value, key)) return undefined;
  if (value[key] === null) return null;
  if (typeof value[key] !== "number" || !Number.isFinite(value[key])) {
    fail(`${path}.${key}`, "number or null");
  }
  return value[key] as number;
}

function defaultObject(value: JsonObject, key: string, path: string): JsonObject {
  if (!has(value, key)) return {};
  return object(value[key], `${path}.${key}`);
}

function optionalObject(value: JsonObject, key: string, path: string): JsonObject | undefined {
  if (!has(value, key)) return undefined;
  return object(value[key], `${path}.${key}`);
}

function optionalStringArray(value: JsonObject, key: string, path: string): string[] | undefined {
  if (!has(value, key)) return undefined;
  return stringArray(value[key], `${path}.${key}`);
}

function stringArray(value: unknown, path: string): string[] {
  if (!Array.isArray(value)) fail(path, "array of strings");
  return value.map((entry, index) => {
    if (typeof entry !== "string") fail(`${path}[${index}]`, "string");
    return entry;
  });
}

function stringMap(value: unknown, path: string): Record<string, string> {
  const record = object(value, path);
  const out: Record<string, string> = {};
  for (const [key, entry] of Object.entries(record)) {
    if (typeof entry !== "string") fail(`${path}.${key}`, "string");
    out[key] = entry;
  }
  return out;
}

function responsesMode(value: JsonObject, key: string, path: string, fallback = "auto"): "auto" | "passthrough" | "convert" {
  const mode = defaultString(value, key, path, fallback);
  if (mode !== "auto" && mode !== "passthrough" && mode !== "convert") {
    fail(`${path}.${key}`, '"auto", "passthrough", or "convert"');
  }
  return mode;
}

function arrayField<T>(
  value: JsonObject,
  key: string,
  path: string,
  decode: PathDecoder<T>,
  fallback?: T[],
): T[] {
  if (!has(value, key)) return fallback ?? [];
  if (!Array.isArray(value[key])) fail(`${path}.${key}`, "array");
  return (value[key] as unknown[]).map((entry, index) => decode(entry, `${path}.${key}[${index}]`));
}

function decodeModelCostAt(value: unknown, path: string): ModelCost {
  const raw = object(value, path);
  const out = { ...raw } as ModelCost;
  out.input = defaultNumber(raw, "input", path, 0);
  out.output = defaultNumber(raw, "output", path, 0);
  out.cacheRead = defaultNumber(raw, "cacheRead", path, 0);
  out.cacheWrite = defaultNumber(raw, "cacheWrite", path, 0);
  if (has(raw, "tiers")) {
    if (!Array.isArray(raw.tiers)) fail(`${path}.tiers`, "array");
    out.tiers = (raw.tiers as unknown[]).map((tier, index) => {
      const decoded = decodeModelCostAt(tier, `${path}.tiers[${index}]`) as ModelCost & { inputTokensAbove: number };
      decoded.inputTokensAbove = requiredNumber(
        object(tier, `${path}.tiers[${index}]`),
        "inputTokensAbove",
        `${path}.tiers[${index}]`,
      );
      return decoded;
    });
  }
  return out;
}

function decodeModelEntryAt(value: unknown, path: string): ModelEntry {
  const raw = object(value, path);
  const out = { ...raw } as ModelEntry;
  out.id = requiredString(raw, "id", path);
  if (out.id.trim() === "") fail(`${path}.id`, "non-empty string");
  out.input = has(raw, "input") ? stringArray(raw.input, `${path}.input`) : [];
  out.contextWindow = defaultNumber(raw, "contextWindow", path, 0);
  out.maxTokens = defaultNumber(raw, "maxTokens", path, 0);
  out.name = optionalString(raw, "name", path);
  out.api = optionalString(raw, "api", path);
  out.baseUrl = optionalString(raw, "baseUrl", path);
  out.reasoning = optionalBoolean(raw, "reasoning", path);
  if (has(raw, "thinkingLevelMap")) out.thinkingLevelMap = stringMapNullable(raw.thinkingLevelMap, `${path}.thinkingLevelMap`);
  if (has(raw, "cost")) out.cost = decodeModelCostAt(raw.cost, `${path}.cost`);
  if (has(raw, "headers")) out.headers = stringMap(raw.headers, `${path}.headers`);
  if (has(raw, "compat")) out.compat = object(raw.compat, `${path}.compat`);
  return out;
}

function stringMapNullable(value: unknown, path: string): Record<string, string | null> {
  const raw = object(value, path);
  const out: Record<string, string | null> = {};
  for (const [key, entry] of Object.entries(raw)) {
    if (entry !== null && typeof entry !== "string") fail(`${path}.${key}`, "string or null");
    out[key] = entry as string | null;
  }
  return out;
}

function decodeUpstreamAt(value: unknown, path: string) {
  const raw = object(value, path);
  const out = { ...raw } as unknown as Upstream;
  out.baseUrl = defaultString(raw, "baseUrl", path, "");
  out.apiKey = defaultString(raw, "apiKey", path, "");
  out.api = optionalString(raw, "api", path);
  if (has(raw, "responsesMode")) out.responsesMode = responsesMode(raw, "responsesMode", path);
  if (has(raw, "headers")) out.headers = stringMap(raw.headers, `${path}.headers`);
  out.weight = optionalNumber(raw, "weight", path);
  out.name = optionalString(raw, "name", path);
  out.requestRetry = optionalNumber(raw, "requestRetry", path);
  out.disableCooling = optionalBoolean(raw, "disableCooling", path);
  if (has(raw, "models")) out.models = arrayField(raw, "models", path, decodeModelEntryAt);
  out.exposedModels = optionalStringArray(raw, "exposedModels", path);
  return out;
}

function decodeProfileAt(value: unknown, path: string): ProviderProfile {
  const raw = object(value, path);
  const out = { ...raw } as ProviderProfile;
  out.api = requiredString(raw, "api", path);
  out.responsesMode = responsesMode(raw, "responsesMode", path);
  out.baseUrl = defaultString(raw, "baseUrl", path, "");
  out.apiKey = defaultString(raw, "apiKey", path, "");
  out.proxy = defaultBoolean(raw, "proxy", path, false);
  out.name = optionalString(raw, "name", path);
  out.oauth = raw.oauth === undefined ? undefined : raw.oauth === "radius" ? "radius" : fail(`${path}.oauth`, '"radius"');
  out.preset = optionalString(raw, "preset", path);
  out.modelsDevProvider = optionalString(raw, "modelsDevProvider", path);
  out.userAgent = optionalString(raw, "userAgent", path);
  if (has(raw, "headers")) out.headers = stringMap(raw.headers, `${path}.headers`);
  if (has(raw, "compat")) out.compat = object(raw.compat, `${path}.compat`);
  if (has(raw, "modelOverrides")) out.modelOverrides = objectOfObjects(raw.modelOverrides, `${path}.modelOverrides`);
  if (has(raw, "modelMap")) out.modelMap = object(raw.modelMap, `${path}.modelMap`);
  out.updatedAt = optionalString(raw, "updatedAt", path);
  if (has(raw, "upstreams")) out.upstreams = arrayField(raw, "upstreams", path, decodeUpstreamAt);
  out.requestRetry = optionalNumber(raw, "requestRetry", path);
  out.disableCooling = optionalBoolean(raw, "disableCooling", path);
  if (has(raw, "requestScopedErrors")) out.requestScopedErrors = arrayOfObjects(raw.requestScopedErrors, `${path}.requestScopedErrors`);
  return out;
}

function objectOfObjects(value: unknown, path: string): Record<string, Record<string, unknown>> {
  const raw = object(value, path);
  const out: Record<string, Record<string, unknown>> = {};
  for (const [key, entry] of Object.entries(raw)) out[key] = object(entry, `${path}.${key}`);
  return out;
}

function arrayOfObjects(value: unknown, path: string): Array<Record<string, unknown>> {
  if (!Array.isArray(value)) fail(path, "array of objects");
  return (value as unknown[]).map((entry, index) => object(entry, `${path}[${index}]`));
}

function decodeSettingsAt(value: unknown, path: string): Settings {
  const raw = object(value, path);
  const out = { ...raw } as unknown as Settings;
  out.writeMode = requiredString(raw, "writeMode", path);
  const source = defaultString(raw, "conversationSource", path, "sessionScan");
  if (source !== "proxy" && source !== "sessionScan" && source !== "off") {
    fail(`${path}.conversationSource`, '"proxy", "sessionScan", or "off"');
  }
  out.conversationSource = source;
  out.injectOpenCodeAttribution = optionalBoolean(raw, "injectOpenCodeAttribution", path);

  const proxy = defaultObject(raw, "proxy", path);
  const circuit = has(proxy, "circuitBreaker") ? object(proxy.circuitBreaker, `${path}.proxy.circuitBreaker`) : {};
  out.proxy = {
    ...proxy,
    host: defaultString(proxy, "host", `${path}.proxy`, "127.0.0.1"),
    port: defaultNumber(proxy, "port", `${path}.proxy`, 43112),
    circuitBreaker: {
      ...circuit,
      enabled: defaultBoolean(circuit, "enabled", `${path}.proxy.circuitBreaker`, true),
      failureThreshold: defaultNumber(circuit, "failureThreshold", `${path}.proxy.circuitBreaker`, 3),
      cooldownSeconds: defaultNumber(circuit, "cooldownSeconds", `${path}.proxy.circuitBreaker`, 60),
    },
  };
  if (has(proxy, "target") && typeof proxy.target !== "string") fail(`${path}.proxy.target`, "string");
  if (has(proxy, "userAgent") && typeof proxy.userAgent !== "string") fail(`${path}.proxy.userAgent`, "string");

  const web = defaultObject(raw, "web", path);
  out.web = {
    ...web,
    host: defaultString(web, "host", `${path}.web`, "127.0.0.1"),
    port: defaultNumber(web, "port", `${path}.web`, 43110),
  };
  return out;
}

function decodeProfilesAt(value: unknown, path: string): Record<string, ProviderProfile> {
  if (value === undefined || value === null) return {};
  const raw = object(value, path);
  const out: Record<string, ProviderProfile> = {};
  for (const [name, profile] of Object.entries(raw)) out[name] = decodeProfileAt(profile, `${path}.${name}`);
  return out;
}

export function decodeSettings(value: unknown): Settings {
  return decodeSettingsAt(value, "settings");
}

export function decodeAppState(value: unknown): AppState {
  const raw = object(value, "state");
  const out = { ...raw } as unknown as AppState;
  if (has(raw, "current")) {
    if (raw.current !== null && typeof raw.current !== "string") fail("state.current", "string or null");
    out.current = raw.current as string | null;
  }
  out.profiles = decodeProfilesAt(raw.profiles, "state.profiles");
  if (!has(raw, "settings")) fail("state.settings", "required object");
  out.settings = decodeSettingsAt(raw.settings, "state.settings");
  return out;
}

function decodePresetAt(value: unknown, path: string): PresetInfo {
  const raw = object(value, path);
  const out = { ...raw } as unknown as PresetInfo;
  out.id = requiredString(raw, "id", path);
  out.name = requiredString(raw, "name", path);
  out.description = defaultString(raw, "description", path, "");
  out.websiteUrl = defaultString(raw, "websiteUrl", path, "");
  out.api = requiredString(raw, "api", path);
  out.baseUrl = defaultString(raw, "baseUrl", path, "");
  out.models = has(raw, "models") ? stringArray(raw.models, `${path}.models`) : [];
  return out;
}

export function decodePresets(value: unknown): PresetInfo[] {
  if (!Array.isArray(value)) fail("presets", "array");
  return (value as unknown[]).map((entry, index) => decodePresetAt(entry, `presets[${index}]`));
}

export function decodeProviderProfile(value: unknown): ProviderProfile {
  return decodeProfileAt(value, "profile");
}

export function decodePresetProfile(value: unknown): ProviderProfile & { name?: string } {
  const raw = object(value, "preset");
  const out = decodeProfileAt(raw, "preset") as ProviderProfile & { name?: string };
  out.name = optionalString(raw, "name", "preset");
  return out;
}

export function decodeProfileDetail(value: unknown): ProfileDetail {
  const raw = object(value, "profile");
  return {
    ...raw,
    name: requiredString(raw, "name", "profile"),
    profile: decodeProfileAt(raw.profile, "profile.profile"),
    providerId: requiredString(raw, "providerId", "profile"),
  };
}

function decodeDoctorAt(value: unknown, path: string): DoctorCheck {
  const raw = object(value, path);
  return { ...raw, ok: requiredBoolean(raw, "ok", path), msg: requiredString(raw, "msg", path) };
}

export function decodeDoctorChecks(value: unknown): DoctorCheck[] {
  if (!Array.isArray(value)) fail("doctor", "array");
  return (value as unknown[]).map((entry, index) => decodeDoctorAt(entry, `doctor[${index}]`));
}

function decodeValidationAt(value: unknown, path: string): ValidationIssue {
  const raw = object(value, path);
  return {
    ...raw,
    level: requiredString(raw, "level", path),
    path: requiredString(raw, "path", path),
    message: requiredString(raw, "message", path),
  };
}

export function decodeValidationIssues(value: unknown): ValidationIssue[] {
  if (!Array.isArray(value)) fail("validation", "array");
  return (value as unknown[]).map((entry, index) => decodeValidationAt(entry, `validation[${index}]`));
}

export function decodeStringArray(value: unknown, path = "response"): string[] {
  return stringArray(value, path);
}

export function decodeDaemonResult(value: unknown): DaemonResult {
  const raw = object(value, "daemon");
  const out = { ...raw } as unknown as DaemonResult;
  out.running = requiredBoolean(raw, "running", "daemon");
  out.message = requiredString(raw, "message", "daemon");
  out.pid = optionalNumber(raw, "pid", "daemon");
  out.host = optionalString(raw, "host", "daemon");
  out.port = optionalNumber(raw, "port", "daemon");
  out.targets = optionalStringArray(raw, "targets", "daemon");
  out.failover = optionalStringArray(raw, "failover", "daemon");
  out.startedAt = optionalNumber(raw, "startedAt", "daemon");
  return out;
}

function decodeGatewayDiff(value: unknown, path: string): GatewayDiff {
  const raw = object(value, path);
  return {
    ...raw,
    added: has(raw, "added") ? stringArray(raw.added, `${path}.added`) : [],
    removed: has(raw, "removed") ? stringArray(raw.removed, `${path}.removed`) : [],
    changed: has(raw, "changed") ? stringArray(raw.changed, `${path}.changed`) : [],
  };
}

function decodePreviewGroupAt(value: unknown, path: string): PreviewGroup {
  const raw = object(value, path);
  const models = arrayField(raw, "models", path, (entry, itemPath) => {
    const item = object(entry, itemPath);
    const status = requiredString(item, "status", itemPath);
    if (status !== "published" && status !== "pending") fail(`${itemPath}.status`, '"published" or "pending"');
    return { ...item, id: requiredString(item, "id", itemPath), status } as PreviewGroup["models"][number];
  });
  return {
    ...raw,
    supplier: requiredString(raw, "supplier", path),
    gatewayProvider: requiredString(raw, "gatewayProvider", path),
    channel: requiredString(raw, "channel", path),
    models,
  };
}

function decodePreviewEnrich(value: unknown, path: string): PreviewEnrich {
  const raw = object(value, path);
  return {
    ...raw,
    enriched: defaultNumber(raw, "enriched", path, 0),
    skipped: defaultNumber(raw, "skipped", path, 0),
    stale: defaultBoolean(raw, "stale", path, false),
    warning: defaultString(raw, "warning", path, ""),
  };
}

export function decodeGatewayPreview(value: unknown): GatewayPreview {
  const raw = object(value, "gatewayPreview");
  const out = { ...raw } as unknown as GatewayPreview;
  if (has(raw, "current") && raw.current !== null) out.current = object(raw.current, "gatewayPreview.current");
  else out.current = null;
  if (has(raw, "proposed") && raw.proposed !== null) out.proposed = object(raw.proposed, "gatewayPreview.proposed");
  else out.proposed = null;
  out.conflicts = has(raw, "conflicts") ? stringArray(raw.conflicts, "gatewayPreview.conflicts") : [];
  out.pending_count = requiredNumber(raw, "pending_count", "gatewayPreview");
  out.diff = has(raw, "diff") ? decodeGatewayDiff(raw.diff, "gatewayPreview.diff") : { added: [], removed: [], changed: [] };
  out.groups = has(raw, "groups")
    ? arrayField(raw, "groups", "gatewayPreview", decodePreviewGroupAt)
    : [];
  out.diagnostics = has(raw, "diagnostics") ? arrayOfObjects(raw.diagnostics, "gatewayPreview.diagnostics") : undefined;
  out.removed = has(raw, "removed") ? stringArray(raw.removed, "gatewayPreview.removed") : [];
  out.enrich = has(raw, "enrich") ? decodePreviewEnrich(raw.enrich, "gatewayPreview.enrich") : undefined;
  return out;
}

function decodeProviderStatsAt(value: unknown, path: string): ProviderStats {
  const raw = withAliases(object(value, path), {
    avgMs: ["avg_ms"],
    totalMs: ["total_ms"],
    promptTokens: ["prompt_tokens"],
    outputTokens: ["output_tokens"],
    cachedTokens: ["cached_tokens"],
    reasoningTokens: ["reasoning_tokens"],
    lastUsed: ["last_used"],
    cacheRate: ["cache_rate"],
  });
  const out = { ...raw } as unknown as ProviderStats;
  out.total = defaultNumber(raw, "total", path, 0);
  out.ok = defaultNumber(raw, "ok", path, 0);
  out.failed = defaultNumber(raw, "failed", path, 0);
  out.retries = defaultNumber(raw, "retries", path, 0);
  out.avgMs = defaultNumber(raw, "avgMs", path, 0);
  out.totalMs = defaultNumber(raw, "totalMs", path, 0);
  out.promptTokens = defaultNumber(raw, "promptTokens", path, 0);
  out.outputTokens = defaultNumber(raw, "outputTokens", path, 0);
  out.cachedTokens = defaultNumber(raw, "cachedTokens", path, 0);
  out.reasoningTokens = defaultNumber(raw, "reasoningTokens", path, 0);
  out.lastUsed = optionalString(raw, "lastUsed", path);
  out.cost = nullableNumber(raw, "cost", path);
  out.cacheRate = optionalString(raw, "cacheRate", path);
  return out;
}

function decodeConversationStatsAt(value: unknown, path: string) {
  const raw = withAliases(object(value, path), {
    conversationId: ["conversation_id"],
    inputTokens: ["input_tokens"],
    outputTokens: ["output_tokens"],
    cachedTokens: ["cached_tokens"],
    reasoningTokens: ["reasoning_tokens"],
    lastActive: ["last_active"],
    cacheRate: ["cache_rate"],
  });
  return {
    ...raw,
    conversationId: requiredString(raw, "conversationId", path),
    name: optionalString(raw, "name", path),
    requests: defaultNumber(raw, "requests", path, 0),
    inputTokens: defaultNumber(raw, "inputTokens", path, 0),
    outputTokens: defaultNumber(raw, "outputTokens", path, 0),
    cachedTokens: defaultNumber(raw, "cachedTokens", path, 0),
    reasoningTokens: defaultNumber(raw, "reasoningTokens", path, 0),
    lastActive: optionalString(raw, "lastActive", path),
    cacheRate: optionalString(raw, "cacheRate", path),
    cost: nullableNumber(raw, "cost", path),
  };
}

function decodeRecentRequestAt(value: unknown, path: string): RecentRequest {
  const raw = withAliases(object(value, path), {
    ok: ["success"],
    promptTokens: ["prompt_tokens"],
    completionTokens: ["completion_tokens"],
    cachedTokens: ["cached_tokens"],
    reasoningTokens: ["reasoning_tokens"],
    totalTokens: ["total_tokens"],
    cacheRate: ["cache_rate"],
    conversationId: ["conversation_id"],
    conversationName: ["conversation_name"],
  });
  const out = { ...raw } as RecentRequest;
  out.ts = nullableString(raw, "ts", path);
  out.provider = nullableString(raw, "provider", path);
  out.model = nullableString(raw, "model", path);
  if (has(raw, "ok")) out.ok = raw.ok === null ? null : requiredBoolean(raw, "ok", path);
  out.status = nullableNumber(raw, "status", path);
  out.error = nullableString(raw, "error", path);
  out.promptTokens = nullableNumber(raw, "promptTokens", path);
  out.completionTokens = nullableNumber(raw, "completionTokens", path);
  out.cachedTokens = nullableNumber(raw, "cachedTokens", path);
  out.reasoningTokens = nullableNumber(raw, "reasoningTokens", path);
  out.totalTokens = nullableNumber(raw, "totalTokens", path);
  out.cacheRate = optionalString(raw, "cacheRate", path);
  out.cost = nullableNumber(raw, "cost", path);
  out.conversationId = nullableString(raw, "conversationId", path);
  out.conversationName = nullableString(raw, "conversationName", path);
  return out;
}

function decodeTokenTotals(value: unknown, path: string): TokenTotals {
  const raw = withAliases(object(value, path), {
    cached: ["cached_tokens"],
    reasoning: ["reasoning_tokens"],
  });
  return {
    ...raw,
    input: defaultNumber(raw, "input", path, 0),
    output: defaultNumber(raw, "output", path, 0),
    total: defaultNumber(raw, "total", path, 0),
    cached: defaultNumber(raw, "cached", path, 0),
    reasoning: defaultNumber(raw, "reasoning", path, 0),
  };
}

export function decodeUsageStats(value: unknown): UsageStats {
  const raw = withAliases(object(value, "stats"), {
    totalRequests: ["total_requests"],
    okRequests: ["ok_requests"],
    failedRequests: ["failed_requests"],
    successRate: ["success_rate"],
    avgLatencyMs: ["avg_latency_ms"],
    byProvider: ["by_provider"],
    byModel: ["by_model"],
    totalTokens: ["total_tokens"],
    cacheHitRate: ["cache_hit_rate"],
    totalCost: ["total_cost"],
    costUnknown: ["cost_unknown"],
    byConversation: ["by_conversation"],
    recentRequests: ["recent_requests", "rows"],
    recentRequestTotal: ["recent_request_total"],
  });
  const out = { ...raw } as UsageStats;
  out.totalRequests = requiredNumber(raw, "totalRequests", "stats");
  out.okRequests = requiredNumber(raw, "okRequests", "stats");
  out.failedRequests = requiredNumber(raw, "failedRequests", "stats");
  out.successRate = requiredString(raw, "successRate", "stats");
  if (!has(raw, "byProvider")) fail("stats.byProvider", "required object");
  const providers = object(raw.byProvider, "stats.byProvider");
  out.byProvider = {};
  for (const [name, provider] of Object.entries(providers)) out.byProvider[name] = decodeProviderStatsAt(provider, `stats.byProvider.${name}`);
  if (has(raw, "byModel")) {
    const models = object(raw.byModel, "stats.byModel");
    const decodedModels: Record<string, ModelStats> = {};
    for (const [name, model] of Object.entries(models)) decodedModels[name] = decodeProviderStatsAt(model, `stats.byModel.${name}`);
    out.byModel = decodedModels;
  }
  out.avgLatencyMs = optionalNumber(raw, "avgLatencyMs", "stats");
  out.cacheHitRate = optionalString(raw, "cacheHitRate", "stats");
  out.totalCost = nullableNumber(raw, "totalCost", "stats");
  out.costUnknown = optionalNumber(raw, "costUnknown", "stats");
  out.totalTokens = has(raw, "totalTokens") ? decodeTokenTotals(raw.totalTokens, "stats.totalTokens") : undefined;
  out.byConversation = has(raw, "byConversation")
    ? arrayField(raw, "byConversation", "stats", decodeConversationStatsAt)
    : undefined;
  out.recentRequests = has(raw, "recentRequests")
    ? arrayField(raw, "recentRequests", "stats", decodeRecentRequestAt)
    : undefined;
  out.recentRequestTotal = optionalNumber(raw, "recentRequestTotal", "stats");
  return out;
}

export function decodeConversationsPage(value: unknown): ConversationsPage {
  const raw = withAliases(object(value, "conversations"), {
    conversations: ["byConversation", "by_conversation"],
  });
  if (!has(raw, "conversations")) fail("conversations.conversations", "required array");
  return {
    ...raw,
    conversations: arrayField(raw, "conversations", "conversations", decodeConversationStatsAt),
    total: requiredNumber(raw, "total", "conversations"),
  };
}

export function decodeConversationRequestsPage(value: unknown): ConversationRequestsPage {
  const raw = withAliases(object(value, "requests"), {
    requests: ["rows"],
  });
  if (!has(raw, "requests")) fail("requests.requests", "required array");
  return {
    ...raw,
    requests: arrayField(raw, "requests", "requests", decodeRecentRequestAt),
    total: requiredNumber(raw, "total", "requests"),
  };
}

export function decodePackage(value: unknown, path = "package"): PackageEntry {
  const raw = object(value, path);
  return {
    ...raw,
    id: requiredString(raw, "id", path),
    name: requiredString(raw, "name", path),
    version: defaultString(raw, "version", path, ""),
    enabled: defaultBoolean(raw, "enabled", path, true),
    installedAt: optionalString(raw, "installedAt", path),
    hasExtensions: optionalBoolean(raw, "hasExtensions", path),
    hasSkills: optionalBoolean(raw, "hasSkills", path),
    hasPrompts: optionalBoolean(raw, "hasPrompts", path),
    hasThemes: optionalBoolean(raw, "hasThemes", path),
  };
}

export function decodePackages(value: unknown): { packages: PackageEntry[] } {
  const raw = object(value, "packages");
  return { ...raw, packages: arrayField(raw, "packages", "packages", decodePackage) };
}

function decodeCcsProviderAt(value: unknown, path: string): CcsProvider {
  const raw = object(value, path);
  return {
    ...raw,
    id: requiredString(raw, "id", path),
    name: defaultString(raw, "name", path, ""),
    appType: defaultString(raw, "appType", path, ""),
    api: requiredString(raw, "api", path),
    baseUrl: defaultString(raw, "baseUrl", path, ""),
    apiKey: defaultString(raw, "apiKey", path, ""),
    models: has(raw, "models") ? stringArray(raw.models, `${path}.models`) : [],
    exists: defaultBoolean(raw, "exists", path, false),
  };
}

export function decodeCcsProviders(value: unknown): { providers: CcsProvider[] } {
  const raw = object(value, "ccswitch");
  return { ...raw, providers: arrayField(raw, "providers", "ccswitch", decodeCcsProviderAt) };
}

function decodeEnrichStatsAt(value: unknown, path: string): EnrichStats {
  const raw = object(value, path);
  return {
    ...raw,
    enriched: defaultNumber(raw, "enriched", path, 0),
    skipped: defaultNumber(raw, "skipped", path, 0),
    failed: defaultNumber(raw, "failed", path, 0),
    warning: nullableString(raw, "warning", path),
  };
}

export function decodeCcsImport(value: unknown): { ok: boolean; imported: number; results: CcsImportResult[] } {
  const raw = object(value, "ccswitch.import");
  const results = arrayOfObjects(has(raw, "results") ? raw.results : [], "ccswitch.import.results").map((entry, index) => ({
    ...entry,
    name: defaultString(entry, "name", `ccswitch.import.results[${index}]`, ""),
    imported: defaultBoolean(entry, "imported", `ccswitch.import.results[${index}]`, false),
    message: defaultString(entry, "message", `ccswitch.import.results[${index}]`, ""),
  }));
  return {
    ...raw,
    ok: requiredBoolean(raw, "ok", "ccswitch.import"),
    imported: defaultNumber(raw, "imported", "ccswitch.import", 0),
    results,
  };
}

export function decodeImportPackages(value: unknown): { ok: boolean; count: number; discovered: number; skipped: number; status: string; message: string; warnings?: string[] } {
  const raw = object(value, "packages.import");
  return {
    ...raw,
    ok: requiredBoolean(raw, "ok", "packages.import"),
    count: defaultNumber(raw, "count", "packages.import", 0),
    discovered: defaultNumber(raw, "discovered", "packages.import", 0),
    skipped: defaultNumber(raw, "skipped", "packages.import", 0),
    status: defaultString(raw, "status", "packages.import", "imported"),
    message: defaultString(raw, "message", "packages.import", ""),
    warnings: has(raw, "warnings") ? stringArray(raw.warnings, "packages.import.warnings") : undefined,
  };
}

export function decodeTestResult(value: unknown): TestResult {
  const raw = object(value, "test");
  return {
    ...raw,
    success: requiredBoolean(raw, "success", "test"),
    message: requiredString(raw, "message", "test"),
    responseTimeMs: optionalNumber(raw, "responseTimeMs", "test"),
  };
}

export function decodeFetchModels(value: unknown): { models: string[]; enrich?: EnrichStats } {
  const raw = object(value, "fetchModels");
  return {
    ...raw,
    models: has(raw, "models") ? stringArray(raw.models, "fetchModels.models") : [],
    enrich: has(raw, "enrich") ? decodeEnrichStatsAt(raw.enrich, "fetchModels.enrich") : undefined,
  };
}

export function decodeUpdateModels(value: unknown): { ok: boolean; backup?: string | null; enrich?: EnrichStats } {
  const raw = object(value, "updateModels");
  return {
    ...raw,
    ok: requiredBoolean(raw, "ok", "updateModels"),
    backup: nullableString(raw, "backup", "updateModels"),
    enrich: has(raw, "enrich") ? decodeEnrichStatsAt(raw.enrich, "updateModels.enrich") : undefined,
  };
}

function decodeUsageWindowAt(value: unknown, path: string): UsageWindow {
  const raw = object(value, path);
  return {
    ...raw,
    percent: defaultNumber(raw, "percent", path, 0),
    status: defaultString(raw, "status", path, ""),
    resetsAt: nullableString(raw, "resetsAt", path),
  };
}

function decodeGoUsageAt(value: unknown, path: string): GoUsage {
  const raw = object(value, path);
  return {
    ...raw,
    rolling: has(raw, "rolling") && raw.rolling !== null ? decodeUsageWindowAt(raw.rolling, `${path}.rolling`) : raw.rolling === null ? null : undefined,
    weekly: has(raw, "weekly") && raw.weekly !== null ? decodeUsageWindowAt(raw.weekly, `${path}.weekly`) : raw.weekly === null ? null : undefined,
    monthly: has(raw, "monthly") && raw.monthly !== null ? decodeUsageWindowAt(raw.monthly, `${path}.monthly`) : raw.monthly === null ? null : undefined,
  };
}

export function decodeCredits(value: unknown): NormalizedCredits {
  const raw = object(value, "credits");
  return {
    ...raw,
    balance: requiredNumber(raw, "balance", "credits"),
    used: requiredNumber(raw, "used", "credits"),
    total: requiredNumber(raw, "total", "credits"),
    remaining: requiredNumber(raw, "remaining", "credits"),
    percent: requiredNumber(raw, "percent", "credits"),
    resetAt: nullableString(raw, "resetAt", "credits"),
    expiry: nullableString(raw, "expiry", "credits"),
    usage: has(raw, "usage") && raw.usage !== null ? decodeGoUsageAt(raw.usage, "credits.usage") : raw.usage === null ? null : undefined,
  };
}

export interface BuildInfo {
  version: string;
  buildTime: string;
  commit: string;
  target: string;
  dirty: string;
  webui?: Record<string, unknown>;
}

export function decodeBuildInfo(value: unknown): BuildInfo {
  const raw = object(value, "buildInfo");
  return {
    ...raw,
    version: requiredString(raw, "version", "buildInfo"),
    buildTime: requiredString(raw, "buildTime", "buildInfo"),
    commit: requiredString(raw, "commit", "buildInfo"),
    target: requiredString(raw, "target", "buildInfo"),
    dirty: requiredString(raw, "dirty", "buildInfo"),
    webui: has(raw, "webui") ? object(raw.webui, "buildInfo.webui") : undefined,
  };
}

export interface GatewayHealth {
  running: boolean;
  mode: string;
  gateway_id: string;
  has_models_file: boolean;
  last_notify: string | null;
  upstreams_total: number;
  message: string;
}

export function decodeGatewayHealth(value: unknown): GatewayHealth {
  const raw = object(value, "gatewayHealth");
  return {
    ...raw,
    running: requiredBoolean(raw, "running", "gatewayHealth"),
    mode: requiredString(raw, "mode", "gatewayHealth"),
    gateway_id: requiredString(raw, "gateway_id", "gatewayHealth"),
    has_models_file: requiredBoolean(raw, "has_models_file", "gatewayHealth"),
    last_notify: nullableString(raw, "last_notify", "gatewayHealth") ?? null,
    upstreams_total: defaultNumber(raw, "upstreams_total", "gatewayHealth", 0),
    message: requiredString(raw, "message", "gatewayHealth"),
  };
}

export function decodeGatewayStart(value: unknown): { running: boolean; mode: string } {
  const raw = object(value, "gatewayStart");
  return {
    ...raw,
    running: requiredBoolean(raw, "running", "gatewayStart"),
    mode: requiredString(raw, "mode", "gatewayStart"),
  };
}

export function decodeMessageList(value: unknown): { messages: string[] } {
  const raw = object(value, "response");
  return { ...raw, messages: has(raw, "messages") ? stringArray(raw.messages, "response.messages") : [] };
}

export function decodeOk(value: unknown): Record<string, unknown> {
  const raw = object(value, "response");
  if (has(raw, "ok")) requiredBoolean(raw, "ok", "response");
  return raw;
}

export function decodeOkResult(value: unknown): { ok: boolean } {
  const raw = object(value, "response");
  return { ...raw, ok: requiredBoolean(raw, "ok", "response") };
}

export function decodeImportResult(value: unknown): { ok: boolean; path?: string; message?: string; backup?: string } {
  const raw = object(value, "response");
  return {
    ...raw,
    ok: has(raw, "ok") ? requiredBoolean(raw, "ok", "response") : true,
    path: optionalString(raw, "path", "response"),
    message: optionalString(raw, "message", "response"),
    backup: optionalString(raw, "backup", "response"),
  };
}

export function decodeGatewayEnvelope(value: unknown): { gateway: unknown } {
  const raw = object(value, "gateway");
  if (!has(raw, "gateway")) fail("gateway.gateway", "required field");
  return { ...raw, gateway: raw.gateway };
}

export function decodeWebUIInfo(value: unknown): { authRequired: boolean } {
  const raw = object(value, "webuiInfo");
  return { ...raw, authRequired: requiredBoolean(raw, "authRequired", "webuiInfo") };
}

export function decodeExportConfig(value: unknown): { path: string } {
  const raw = object(value, "config.export");
  return { ...raw, path: requiredString(raw, "path", "config.export") };
}

export function decodeImportConfig(value: unknown): { message: string } {
  const raw = object(value, "config.import");
  return { ...raw, message: requiredString(raw, "message", "config.import") };
}

export function decodeRestoreConfig(value: unknown): { backup: string } {
  const raw = object(value, "config.restore");
  return { ...raw, backup: requiredString(raw, "backup", "config.restore") };
}

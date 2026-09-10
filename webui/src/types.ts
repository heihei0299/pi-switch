// Type mirror of Go config structs in `internal/config/config.go`.
// `internal/config/config.go` is the source of truth —
// keep these in sync when they change (manual sync, no OpenAPI generation; gateway publish diff guards drift indirectly).
// (Future option noted in WEBUI_GUIDE.md: auto-generate via typeshare/ts-rs.)

export interface ModelCost {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  tiers?: Array<ModelCost & { inputTokensAbove: number }>;
  [key: string]: unknown;
}

export interface ModelEntry {
  id: string;
  name?: string;
  api?: string;
  baseUrl?: string;
  reasoning?: boolean;
  thinkingLevelMap?: Record<string, string | null>;
  input: string[];
  contextWindow: number;
  maxTokens: number;
  cost?: ModelCost;
  headers?: Record<string, string>;
  compat?: Record<string, unknown>;
  [key: string]: unknown;
}

export type ResponsesMode = "auto" | "passthrough" | "convert";

export interface Upstream {
  baseUrl: string;
  apiKey: string;
  api?: string;
  responsesMode?: ResponsesMode;
  headers?: Record<string, string>;
  weight?: number;
  name?: string;
  requestRetry?: number;
  disableCooling?: boolean;
  /** 渠道分区模型池（与 Go Upstream.models 同步） */
  models?: ModelEntry[];
  /** 渠道分区暴露集（与 Go Upstream.exposedModels 同步） */
  exposedModels?: string[];
}

export interface ProviderProfile {
  name?: string;
  api: string;
  responsesMode?: ResponsesMode;
  baseUrl: string;
  apiKey: string;
  /** 多上游配置（进程隔离后独立调度）。空时回退到单 baseUrl/apiKey/headers，兼容旧字段 */
  upstreams?: Upstream[];
  oauth?: "radius";
  preset?: string;
  /** 模型目录 provider 映射（对应模型目录的 provider key，如 "openai"）；显式值优先，未填时按 preset 推断，推断失败跳过模型元数据 enrich */
  modelsDevProvider?: string;
  headers?: Record<string, string>;
  authHeader?: boolean;
  compat?: Record<string, unknown>;
  modelOverrides?: Record<string, Record<string, unknown>>;
  proxy: boolean;
  updatedAt?: string;
  modelMap?: Record<string, unknown>;
  userAgent?: string;
  requestRetry?: number;
  disableCooling?: boolean;
  requestScopedErrors?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export function hasUpstreams(profile: ProviderProfile): boolean {
  return Array.isArray(profile.upstreams) && profile.upstreams.length > 0;
}

export function resolvedUpstreams(profile: ProviderProfile): Upstream[] {
  if (hasUpstreams(profile)) return profile.upstreams!;
  if (profile.baseUrl || profile.apiKey || profile.headers) {
    return [{ baseUrl: profile.baseUrl, apiKey: profile.apiKey, headers: profile.headers }];
  }
  return [];
}

export interface CircuitBreakerSettings {
  enabled: boolean;
  failureThreshold: number;
  cooldownSeconds: number;
}

export interface ProxySettings {
  host: string;
  port: number;
  target?: string;
  userAgent?: string;
  circuitBreaker: CircuitBreakerSettings;
}

export interface WebSettings {
  host: string;
  port: number;
}

export interface Settings {
  writeMode: string;
  injectOpenCodeAttribution?: boolean;
  language?: string | null;
  proxy: ProxySettings;
  web: WebSettings;
  conversationSource: "proxy" | "sessionScan" | "off";
}

export interface AppState {
  current?: string | null;
  profiles: Record<string, ProviderProfile>;
  settings: Settings;
}

export interface PresetInfo {
  id: string;
  name: string;
  description: string;
  websiteUrl: string;
  api: string;
  baseUrl: string;
  models: string[];
}

export interface DoctorCheck {
  ok: boolean;
  msg: string;
}

export interface ValidationIssue {
  level: string;
  path: string;
  message: string;
}

export interface DaemonResult {
  running: boolean;
  pid?: number;
  host?: string;
  port?: number;
  targets?: string[];
  failover?: string[];
  startedAt?: number;
  message: string;
}

export interface TestResult {
  success: boolean;
  message: string;
  responseTimeMs?: number;
}

export interface EnrichStats {
  enriched: number;
  skipped: number;
  failed: number;
  warning?: string | null;
}

export interface ProfileDetail {
  name: string;
  profile: ProviderProfile;
  providerId: string;
}

export interface TokenTotals {
  input: number;
  output: number;
  total: number;
  cached: number;
  reasoning: number;
}

export interface ProviderStats {
  total: number;
  ok: number;
  failed: number;
  retries: number;
  avgMs: number;
  totalMs: number;
  lastUsed?: string;
  promptTokens: number;
  outputTokens: number;
  cachedTokens: number;
  reasoningTokens: number;
  cost?: number | null;
  cacheRate?: string;
}

export interface ConversationStats {
  conversationId: string;
  name?: string;
  requests: number;
  inputTokens: number;
  outputTokens: number;
  cachedTokens: number;
  reasoningTokens: number;
  lastActive?: string;
  cacheRate?: string;
  cost?: number | null;
}

export interface ConversationsPage {
  conversations: ConversationStats[];
  total: number;
}

export interface ConversationRequestsPage {
  requests: RecentRequest[];
  total: number;
}

export interface RecentRequest {
  ts?: string | null;
  provider?: string | null;
  model?: string | null;
  ok?: boolean | null;
  status?: number | null;
  error?: string | null;
  promptTokens?: number | null;
  completionTokens?: number | null;
  cachedTokens?: number | null;
  reasoningTokens?: number | null;
  totalTokens?: number | null;
  cacheRate?: string;
  cost?: number | null;
  conversationId?: string | null;
  conversationName?: string | null;
}

export interface ModelStats {
  total: number;
  ok: number;
  failed?: number;
  promptTokens?: number;
  outputTokens?: number;
  cachedTokens?: number;
  reasoningTokens?: number;
  cost?: number | null;
  cacheRate?: string;
}

export interface UsageStats {
  totalRequests: number;
  okRequests: number;
  failedRequests: number;
  successRate: string;
  avgLatencyMs?: number;
  byProvider: Record<string, ProviderStats>;
  byModel?: Record<string, ModelStats>;
  totalTokens?: TokenTotals;
  cacheHitRate?: string;
  totalCost?: number | null;
  costUnknown?: number;
  byConversation?: ConversationStats[];
  recentRequests?: RecentRequest[];
  recentRequestTotal?: number;
  [key: string]: unknown;
}

// 网关预览分组（与 Go gateway.PreviewGroup 同步）：按供应商/渠道分组的已暴露候选。
export interface PreviewGroupItem {
  id: string;
  status: "published" | "pending";
}

export interface PreviewGroup {
  supplier: string;
  gatewayProvider: string;
  channel: string;
  models: PreviewGroupItem[];
}

export interface GatewaySelection {
  supplier: string;
  channel: string;
  model: string;
}

export interface GatewayDiff {
  added: string[];
  removed: string[];
  changed: string[];
}

export interface GatewayPreview {
  current: Record<string, unknown> | null;
  proposed: Record<string, unknown> | null;
  conflicts: string[];
  diagnostics?: Array<Record<string, unknown>>;
  pending_count: number;
  diff: GatewayDiff;
  groups: PreviewGroup[];
  removed: string[];
  enrich?: PreviewEnrich;
}

// 网关预览 enrich 摘要（与后端 enrich gin.H 同步）。
export interface PreviewEnrich {
  enriched: number;
  skipped: number;
  stale: boolean;
  warning: string;
}
export interface PackageEntry {
  id: string;
  name: string;
  version: string;
  enabled: boolean;
  description?: string;
  homepage?: string;
  origin?: string;
  installedAt?: string;
  hasExtensions?: boolean;
  hasSkills?: boolean;
  hasPrompts?: boolean;
  hasThemes?: boolean;
}

export interface PackageImportResult {
  ok: boolean;
  count: number;
  discovered: number;
  skipped: number;
  status: "imported" | "empty" | "not_found" | string;
  message: string;
  warnings?: string[];
}

export interface CcsProvider {
  id: string;
  name: string;
  appType: string;
  api: string;
  baseUrl: string;
  apiKey: string;
  models: string[];
  exists: boolean;
}

export interface CcsImportResult {
  name: string;
  imported: boolean;
  message: string;
}

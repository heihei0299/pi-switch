import type { ModelCost, ModelEntry } from "../types";

// ─── Pi thinking levels ──────────────────────────────────────────────
export const PI_THINKING_LEVELS = [
  "off",
  "minimal",
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
] as const;
export type PiThinkingLevel = (typeof PI_THINKING_LEVELS)[number];
export type PiThinkingLevelMap = Partial<Record<PiThinkingLevel, string | null>>;

export function isPiThinkingLevelMap(value: unknown): value is PiThinkingLevelMap {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
  const obj = value as Record<string, unknown>;
  for (const [k, v] of Object.entries(obj)) {
    if (!(PI_THINKING_LEVELS as readonly string[]).includes(k)) return false;
    if (v !== null && typeof v !== "string") return false;
  }
  return true;
}

export type PiThinkingLevelMode = "default" | "unsupported" | "value";

export function thinkingLevelMode(
  map: PiThinkingLevelMap,
  level: PiThinkingLevel,
): PiThinkingLevelMode {
  if (!Object.prototype.hasOwnProperty.call(map, level)) return "default";
  return (map as Record<string, unknown>)[level] === null ? "unsupported" : "value";
}

// ─── Input helpers ──────────────────────────────────────────────────
export function supportsImageInput(value: unknown): boolean {
  return Array.isArray(value) && value.includes("image");
}

export function withImageInput(value: unknown, enabled: boolean): string[] {
  const additional = Array.isArray(value)
    ? (value as unknown[]).filter(
        (item): item is string =>
          typeof item === "string" && item !== "text" && item !== "image",
      )
    : [];
  return ["text", ...(enabled ? ["image"] : []), ...new Set(additional)];
}

// ─── Validation helpers ─────────────────────────────────────────────
export function positiveNumber(
  value: string,
  errorMessage: string,
): number {
  const parsed = Number(value);
  if (value.trim() === "" || !Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(errorMessage);
  }
  return parsed;
}

export function validateAbsoluteHttpUrl(value: string, errorMessage: string): void {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error(errorMessage);
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error(errorMessage);
  }
}

const SUPPORTED_APIS = ["openai-completions", "openai-responses", "anthropic-messages", "google-generative-ai"] as const;

export interface ValidateModelResult {
  ok: boolean;
  error?: string;
  value?: Record<string, unknown>;
}
export interface ValidateModelsResult {
  ok: boolean;
  error?: string;
  value?: Record<string, unknown>[];
}
export interface ValidateProfileResult {
  ok: boolean;
  error?: string;
  value?: Record<string, unknown>;
}

function isPositiveNumber(value: unknown): boolean {
  if (typeof value === "number") return Number.isFinite(value) && value > 0;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    return Number.isFinite(n) && n > 0;
  }
  return false;
}

function normalizePositiveNumber(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const n = Number(value);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return undefined;
}

function validateCostShape(cost: unknown): string | null {
  if (cost === undefined) return null;
  if (!cost || typeof cost !== "object" || Array.isArray(cost)) return "cost must be an object";
  const co = cost as Record<string, unknown>;
  for (const k of ["input", "output", "cacheRead", "cacheWrite"]) {
    if (k in co && typeof co[k] !== "number") return `cost.${k} must be a number`;
  }
  if ("tiers" in co && co.tiers !== undefined) {
    if (!Array.isArray(co.tiers)) return "cost.tiers must be an array";
    for (let i = 0; i < (co.tiers as unknown[]).length; i++) {
      const t = (co.tiers as Record<string, unknown>[])[i] as Record<string, unknown>;
      if (!t || typeof t !== "object" || Array.isArray(t)) return `cost.tiers[${i}] must be an object`;
      if (typeof t.inputTokensAbove !== "number") return `cost.tiers[${i}].inputTokensAbove must be a number`;
      for (const kk of ["input", "output", "cacheRead", "cacheWrite"]) {
        if (kk in t && typeof t[kk] !== "number") return `cost.tiers[${i}].${kk} must be a number`;
      }
    }
  }
  return null;
}

export function validateModelEntry(entry: unknown): ValidateModelResult {
  if (!entry || typeof entry !== "object" || Array.isArray(entry)) {
    return { ok: false, error: "model must be an object" };
  }
  const obj = entry as Record<string, unknown>;
  const rawId = obj.id;
  if (typeof rawId !== "string" || !rawId.trim()) {
    return { ok: false, error: "model.id must not be empty" };
  }
  const id = rawId.trim();

  // contextWindow / context_window
  const rawCW = hasOwn(obj, "contextWindow") ? obj.contextWindow : hasOwn(obj, "context_window") ? (obj as any).context_window : undefined;
  if (rawCW !== undefined && !isPositiveNumber(rawCW)) {
    return { ok: false, error: "model.contextWindow must be a positive number" };
  }
  const rawMT = hasOwn(obj, "maxTokens") ? obj.maxTokens : hasOwn(obj, "max_tokens") ? (obj as any).max_tokens : undefined;
  if (rawMT !== undefined && !isPositiveNumber(rawMT)) {
    return { ok: false, error: "model.maxTokens must be a positive number" };
  }

  if ("cost" in obj) {
    const ce = validateCostShape(obj.cost);
    if (ce) return { ok: false, error: ce };
  }

  // Build normalized value: passthrough + controlled fields in camelCase
  const passthrough = objectWithout(obj, CONTROLLED_MODEL_KEYS);
  // Remove legacy snake keys from passthrough if present
  const legacyKeys = ["context_window", "max_tokens"] as const;
  for (const lk of legacyKeys) delete (passthrough as any)[lk];
  const normalized: Record<string, unknown> = { ...passthrough, id };
  if (hasOwn(obj, "name") && typeof obj.name === "string") normalized.name = obj.name;
  else if (hasOwn(obj, "name")) normalized.name = obj.name;
  if (hasOwn(obj, "reasoning")) normalized.reasoning = obj.reasoning;
  if (hasOwn(obj, "input")) normalized.input = obj.input;
  if (rawCW !== undefined) normalized.contextWindow = normalizePositiveNumber(rawCW);
  if (rawMT !== undefined) normalized.maxTokens = normalizePositiveNumber(rawMT);
  if (hasOwn(obj, "thinkingLevelMap")) normalized.thinkingLevelMap = obj.thinkingLevelMap;
  if (hasOwn(obj, "cost")) normalized.cost = obj.cost;
  // Preserve headers/compat etc already in passthrough
  return { ok: true, value: normalized };
}

export function validateModelsJson(text: string): ValidateModelsResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    return { ok: false, error: `Invalid JSON: ${msg}` };
  }
  if (!Array.isArray(parsed)) {
    return { ok: false, error: "models must be an array" };
  }
  const out: Record<string, unknown>[] = [];
  const seen = new Set<string>();
  for (let i = 0; i < parsed.length; i++) {
    const entry = parsed[i];
    const res = validateModelEntry(entry);
    if (!res.ok) {
      return { ok: false, error: res.error ? `models[${i}].${res.error}` : `models[${i}] invalid` };
    }
    const id = (res.value as Record<string, unknown>).id as string;
    if (seen.has(id)) {
      return { ok: false, error: `duplicate model id: ${id}` };
    }
    seen.add(id);
    out.push(res.value as Record<string, unknown>);
  }
  return { ok: true, value: out };
}

export function validateProfileJson(text: string): ValidateProfileResult {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    return { ok: false, error: `Invalid JSON: ${msg}` };
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { ok: false, error: "profile must be an object" };
  }
  const obj = value as Record<string, unknown>;
  const api = obj.api;
  if (typeof api !== "string" || !api) {
    return { ok: false, error: "profile.api is required" };
  }
  if (!(SUPPORTED_APIS as readonly string[]).includes(api)) {
    return { ok: false, error: `profile.api is not supported: ${api}` };
  }
  // baseUrl is required if not using upstreams? For simplicity require baseUrl always as before, but allow upstreams to supplement
  const baseUrl = obj.baseUrl;
  if (typeof baseUrl !== "string" || !baseUrl) {
    return { ok: false, error: "profile.baseUrl is required" };
  }
  if (!baseUrl.startsWith("http://") && !baseUrl.startsWith("https://")) {
    return { ok: false, error: "profile.baseUrl must start with http:// or https://" };
  }
  if (hasOwn(obj, "upstreams") && obj.upstreams !== undefined) {
    if (!Array.isArray(obj.upstreams)) return { ok: false, error: "profile.upstreams must be an array" };
    for (let i = 0; i < (obj.upstreams as unknown[]).length; i++) {
      const u = (obj.upstreams as Record<string, unknown>[])[i] as Record<string, unknown>;
      if (!u || typeof u !== "object" || Array.isArray(u)) return { ok: false, error: `profile.upstreams[${i}] must be an object` };
      const ub = u.baseUrl as unknown;
      if (ub !== undefined && (typeof ub !== "string" || !String(ub).startsWith("http"))) {
        if (typeof ub === "string" && ub && !ub.startsWith("http://") && !ub.startsWith("https://"))
          return { ok: false, error: `profile.upstreams[${i}].baseUrl must start with http:// or https://` };
      }
    }
  }
  return { ok: true, value: obj };
}

// ─── Model draft (UI string-backed) ────────────────────────────────
export interface ModelDraft {
  key: string;
  id: string;
  name: string;
  hasName: boolean;
  reasoning: boolean;
  hasReasoning: boolean;
  input: unknown;
  hasInput: boolean;
  contextWindow: string;
  hasContextWindow: boolean;
  maxTokens: string;
  hasMaxTokens: boolean;
  thinkingLevelMap: unknown;
  hasThinkingLevelMap: boolean;
  cost?: ModelCost;
  hasCost: boolean;
  passthrough: Record<string, unknown>;
}

function optionalText(value: unknown): string {
  return typeof value === "string" ? value : "";
}
function optionalNumberText(value: unknown): string {
  return typeof value === "number" && Number.isFinite(value) ? String(value) : "";
}
function hasOwn(value: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(value, key);
}
function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}
function objectWithout(
  value: Record<string, unknown>,
  denied: Set<string>,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(value).filter(([k]) => !denied.has(k)),
  );
}

export const CONTROLLED_MODEL_KEYS = new Set([
  "id",
  "name",
  "reasoning",
  "input",
  "contextWindow",
  "maxTokens",
  "thinkingLevelMap",
  "cost",
]);

export function modelDraft(value: unknown, opts: { key?: string } = {}): ModelDraft {
  const model = asObject(value);
  const costRaw = model.cost;
  let cost: ModelCost | undefined;
  if (costRaw && typeof costRaw === "object" && !Array.isArray(costRaw)) {
    const co = costRaw as Record<string, unknown>;
    cost = {
      input: typeof co.input === "number" ? co.input : 0,
      output: typeof co.output === "number" ? co.output : 0,
      cacheRead: typeof co.cacheRead === "number" ? co.cacheRead : 0,
      cacheWrite: typeof co.cacheWrite === "number" ? co.cacheWrite : 0,
      tiers: Array.isArray(co.tiers) ? (co.tiers as any) : undefined,
    } as ModelCost;
  }
  return {
    key: opts.key ?? (typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2)),
    id: optionalText(model.id),
    name: optionalText(model.name),
    hasName: hasOwn(model, "name"),
    reasoning: model.reasoning === true,
    hasReasoning: hasOwn(model, "reasoning"),
    input: Array.isArray(model.input) ? model.input : ["text"],
    hasInput: hasOwn(model, "input"),
    contextWindow: optionalNumberText(model.contextWindow ?? (model as any).context_window),
    hasContextWindow: hasOwn(model, "contextWindow") || hasOwn(model, "context_window"),
    maxTokens: optionalNumberText(model.maxTokens ?? (model as any).max_tokens),
    hasMaxTokens: hasOwn(model, "maxTokens") || hasOwn(model, "max_tokens"),
    thinkingLevelMap: model.thinkingLevelMap,
    hasThinkingLevelMap: hasOwn(model, "thinkingLevelMap"),
    cost,
    hasCost: hasOwn(model, "cost"),
    passthrough: objectWithout(model, CONTROLLED_MODEL_KEYS),
  };
}

export function newModelDraft(): ModelDraft {
  return {
    key: typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2),
    id: "",
    name: "",
    hasName: true,
    reasoning: false,
    hasReasoning: true,
    input: ["text"],
    hasInput: true,
    contextWindow: "",
    hasContextWindow: true,
    maxTokens: "",
    hasMaxTokens: true,
    thinkingLevelMap: undefined,
    hasThinkingLevelMap: false,
    cost: undefined,
    hasCost: false,
    passthrough: {},
  };
}

export function modelPreview(draft: ModelDraft): Record<string, unknown> {
  const displayName = draft.name.trim();
  const previewNumber = (value: string): number | string | undefined => {
    if (!value.trim()) return undefined;
    const parsed = Number(value);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : value;
  };
  const cw = previewNumber(draft.contextWindow);
  const mt = previewNumber(draft.maxTokens);
  return {
    ...draft.passthrough,
    id: draft.id,
    ...(draft.hasName ? { name: displayName } : {}),
    ...(draft.hasReasoning ? { reasoning: draft.reasoning } : {}),
    ...(draft.hasInput ? { input: withImageInput(draft.input, supportsImageInput(draft.input)) } : {}),
    ...(draft.hasContextWindow && cw !== undefined ? { contextWindow: cw } : {}),
    ...(draft.hasMaxTokens && mt !== undefined ? { maxTokens: mt } : {}),
    ...(draft.hasThinkingLevelMap ? { thinkingLevelMap: draft.thinkingLevelMap } : {}),
    ...(draft.hasCost && draft.cost ? { cost: draft.cost } : {}),
  };
}

export function draftFromEntry(entry: ModelEntry, key?: string): ModelDraft {
  return modelDraft(entry as unknown as Record<string, unknown>, { key });
}

export function entryFromDraft(draft: ModelDraft): ModelEntry {
  const preview = modelPreview(draft);
  return preview as unknown as ModelEntry;
}

// ─── Gateway building (for live preview) ───────────────────────────
export function buildGatewayPreview(
  profile: Partial<Record<string, unknown>> & { api?: string; baseUrl?: string; apiKey?: string },
  drafts: ModelDraft[],
  opts: {
    headers?: Record<string, string>;
    compat?: Record<string, unknown>;
    providerPassthrough?: Record<string, unknown>;
  } = {},
): Record<string, unknown> {
  const models = drafts.map(modelPreview);
  return {
    ...(opts.providerPassthrough ?? {}),
    ...(profile.name ? { name: profile.name } : {}),
    api: profile.api ?? "openai-completions",
    baseUrl: profile.baseUrl ?? "http://127.0.0.1:43112/v1",
    ...(profile.apiKey ? { apiKey: profile.apiKey } : {}),
    ...(opts.headers && Object.keys(opts.headers).length > 0 ? { headers: opts.headers } : {}),
    ...(opts.compat && Object.keys(opts.compat).length > 0 ? { compat: opts.compat } : {}),
    models,
  };
}

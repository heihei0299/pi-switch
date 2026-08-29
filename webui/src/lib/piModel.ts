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

export interface GatewayDiff {
  added: string[];
  removed: string[];
  changed: string[];
}

export function diffGateway(current: Record<string, unknown> | null, proposed: Record<string, unknown>): GatewayDiff {
  if (!current) {
    return { added: Object.keys(proposed), removed: [], changed: [] };
  }
  const curKeys = new Set(Object.keys(current));
  const propKeys = new Set(Object.keys(proposed));
  const added: string[] = [];
  const removed: string[] = [];
  const changed: string[] = [];

  for (const k of propKeys) {
    if (!curKeys.has(k)) added.push(k);
    else {
      const a = JSON.stringify((current as Record<string, unknown>)[k]);
      const b = JSON.stringify((proposed as Record<string, unknown>)[k]);
      if (a !== b) changed.push(k);
    }
  }
  for (const k of curKeys) {
    if (!propKeys.has(k)) removed.push(k);
  }
  return { added, removed, changed };
}

export function detectConflicts(
  current: Record<string, unknown> | null,
  proposed: Record<string, unknown> | null,
  conflicts: string[],
): string[] {
  if (!current || !proposed) return [];
  const diff = diffGateway(current, proposed);
  return conflicts.filter((k) => diff.changed.includes(k));
}

export interface ValidateResult {
  ok: boolean;
  error?: string;
  value?: Record<string, unknown>;
}

const SUPPORTED_APIS = ["openai-completions", "openai-responses", "anthropic-messages", "google-generative-ai"];

export function validateGatewayJson(text: string): ValidateResult {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    return { ok: false, error: `Invalid JSON: ${msg}` };
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return { ok: false, error: "gateway must be an object" };
  }
  const obj = value as Record<string, unknown>;
  const api = obj["api"];
  if (typeof api !== "string" || !api) {
    return { ok: false, error: "gateway.api is required" };
  }
  if (!SUPPORTED_APIS.includes(api)) {
    return { ok: false, error: `gateway.api is not supported: ${api}` };
  }
  const baseUrl = obj["baseUrl"];
  if (typeof baseUrl !== "string" || !baseUrl) {
    return { ok: false, error: "gateway.baseUrl is required" };
  }
  if (!baseUrl.startsWith("http://") && !baseUrl.startsWith("https://")) {
    return { ok: false, error: "gateway.baseUrl must start with http:// or https://" };
  }
  const models = obj["models"];
  if (!Array.isArray(models)) {
    return { ok: false, error: "gateway.models must be an array" };
  }
  for (let i = 0; i < models.length; i++) {
    const m = models[i] as Record<string, unknown>;
    const id = m?.["id"];
    if (typeof id !== "string" || !id.trim()) {
      return { ok: false, error: `gateway.models[${i}].id must not be empty` };
    }
  }
  return { ok: true, value: obj };
}

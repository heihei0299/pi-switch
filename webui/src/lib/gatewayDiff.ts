export interface GatewayDiff {
  added: string[];
  removed: string[];
  changed: string[];
}

export function diffGateway(current: Record<string, unknown> | null, proposed: Record<string, unknown>): GatewayDiff {
  if (!current) {
    return { added: Object.keys(proposed), removed: [], changed: [] };
  }
  // Detect providers map (per-channel bare): each value has models array
  const isProvidersMap = (obj: Record<string, unknown>) => {
    const vals = Object.values(obj);
    return vals.length > 0 && vals.some((v) => v && typeof v === "object" && !Array.isArray(v) && "models" in (v as Record<string, unknown>));
  };
  const curIsProviders = isProvidersMap(current);
  const propIsProviders = isProvidersMap(proposed);
  if (curIsProviders || propIsProviders) {
    const curProvs = current as Record<string, Record<string, unknown>>;
    const propProvs = proposed as Record<string, Record<string, unknown>>;
    const added: string[] = [];
    const removed: string[] = [];
    const changed: string[] = [];
    const curKeys = new Set(Object.keys(curProvs));
    const propKeys = new Set(Object.keys(propProvs));
    for (const k of propKeys) {
      if (!curKeys.has(k)) {
        const entry = propProvs[k] as unknown as { models?: unknown[] };
        const models = entry?.models;
        if (Array.isArray(models)) {
          for (const m of models) {
            const id = (m as Record<string, unknown>)?.["id"] as string | undefined;
            if (id) added.push(`${k}/${id}`);
            else added.push(k);
          }
          if (models.length === 0) added.push(k);
        } else {
          added.push(k);
        }
      }
    }
    for (const k of curKeys) {
      if (!propKeys.has(k)) {
        const entry = curProvs[k] as unknown as { models?: unknown[] };
        const models = entry?.models;
        if (Array.isArray(models)) {
          for (const m of models) {
            const id = (m as Record<string, unknown>)?.["id"] as string | undefined;
            if (id) removed.push(`${k}/${id}`);
            else removed.push(k);
          }
          if (models.length === 0) removed.push(k);
        } else {
          removed.push(k);
        }
      }
    }
    for (const k of propKeys) {
      if (!curKeys.has(k)) continue;
      const curEntry = curProvs[k] as Record<string, unknown>;
      const propEntry = propProvs[k] as Record<string, unknown>;
      const curCopy = { ...curEntry } as Record<string, unknown>;
      const propCopy = { ...propEntry } as Record<string, unknown>;
      delete curCopy["models"];
      delete propCopy["models"];
      if (JSON.stringify(curCopy) !== JSON.stringify(propCopy)) {
        changed.push(k);
      }
      const curModels = (curEntry["models"] as unknown[] | undefined) || [];
      const propModels = (propEntry["models"] as unknown[] | undefined) || [];
      const curById = new Map<string, unknown>();
      const propById = new Map<string, unknown>();
      for (const m of curModels) {
        const id = (m as Record<string, unknown>)?.["id"] as string | undefined;
        if (id) curById.set(id, m);
      }
      for (const m of propModels) {
        const id = (m as Record<string, unknown>)?.["id"] as string | undefined;
        if (id) propById.set(id, m);
      }
      for (const [id, propM] of propById) {
        if (!curById.has(id)) {
          added.push(`${k}/${id}`);
        } else {
          const curM = curById.get(id);
          if (JSON.stringify(curM) !== JSON.stringify(propM)) {
            changed.push(`${k}/${id}`);
          }
        }
      }
      for (const [id] of curById) {
        if (!propById.has(id)) {
          removed.push(`${k}/${id}`);
        }
      }
    }
    return { added, removed, changed };
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

// Single source of truth for the providers this UI maintains. Mirrors the
// backend predicate internal/gateway.IsFixedGatewayProvider.
const FIXED_GATEWAY_PROVIDERS = ["pi-switch-res", "pi-switch-chat"] as const;

// Each fixed provider only accepts one API shape.
const FIXED_GATEWAY_API: Record<(typeof FIXED_GATEWAY_PROVIDERS)[number], string> = {
  "pi-switch-res": "openai-responses",
  "pi-switch-chat": "openai-completions",
};

export function isFixedGatewayProvider(key: string): boolean {
  return (FIXED_GATEWAY_PROVIDERS as readonly string[]).includes(key);
}

export function filterFixedGatewayProviders(providers: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(providers).filter(([k]) => isFixedGatewayProvider(k)));
}

// Keep diff entries that belong to a fixed provider. The backend emits bare
// provider keys, but the shared GatewayDiff shape also allows composite
// `provider/model` entries, so match `key` or `key/...` instead of splitting
// on "/" (model ids may legitimately contain slashes for wild providers).
export function filterFixedGatewayDiff(diff: GatewayDiff): GatewayDiff {
  const keepFixed = (entry: string) =>
    FIXED_GATEWAY_PROVIDERS.some((provider) => entry === provider || entry.startsWith(`${provider}/`));
  return {
    added: diff.added.filter(keepFixed),
    removed: diff.removed.filter(keepFixed),
    changed: diff.changed.filter(keepFixed),
  };
}

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
  // Providers wrapper: { providers: { ... } }
  // ponytail: UI彻底不读野生第三方，只校验网关维护的固定集，后端合并保留野生的。
  if ("providers" in obj) {
    const provs = obj["providers"];
    if (typeof provs !== "object" || provs === null || Array.isArray(provs)) {
      return { ok: false, error: "gateway.providers must be an object" };
    }
    const filtered: Record<string, unknown> = {};
    for (const [key, entry] of Object.entries(provs as Record<string, unknown>)) {
      if (!isFixedGatewayProvider(key)) continue;
      if (typeof entry !== "object" || entry === null || Array.isArray(entry)) {
        return { ok: false, error: `gateway.providers[${key}] must be object` };
      }
      const rec = entry as Record<string, unknown>;
      const api = rec["api"];
      if (typeof api !== "string" || !api) {
        return { ok: false, error: `gateway.providers[${key}].api is required` };
      }
      if (!SUPPORTED_APIS.includes(api as string)) {
        return { ok: false, error: `gateway.providers[${key}].api is not supported: ${api}` };
      }
      const requiredApi = FIXED_GATEWAY_API[key as (typeof FIXED_GATEWAY_PROVIDERS)[number]];
      if (requiredApi && api !== requiredApi) {
        return { ok: false, error: `gateway.providers[${key}].api must be ${requiredApi}` };
      }
      const baseUrl = rec["baseUrl"];
      if (typeof baseUrl !== "string" || !baseUrl) {
        return { ok: false, error: `gateway.providers[${key}].baseUrl is required` };
      }
      if (!baseUrl.startsWith("http://") && !baseUrl.startsWith("https://")) {
        return { ok: false, error: `gateway.providers[${key}].baseUrl must start with http:// or https://` };
      }
      const models = rec["models"];
      if (!Array.isArray(models)) {
        return { ok: false, error: `gateway.providers[${key}].models must be an array` };
      }
      for (let i = 0; i < models.length; i++) {
        const m = models[i] as Record<string, unknown>;
        const id = m?.["id"];
        if (typeof id !== "string" || !id.trim()) {
          return { ok: false, error: `gateway.providers[${key}].models[${i}].id must not be empty` };
        }
        if ((id as string).includes("/")) {
          return { ok: false, error: `gateway.providers[${key}].models[${i}].id must not contain "/"` };
        }
      }
      filtered[key] = entry;
    }
    return { ok: true, value: { ...obj, providers: filtered } };
  }
  return { ok: false, error: "gateway.providers is required" };
}

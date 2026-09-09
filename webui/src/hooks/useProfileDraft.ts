import { useCallback, useReducer } from "react";
import type { ModelEntry, ProviderProfile, Upstream } from "../types";
import { validateModelsJson, validateProfileJson } from "../lib/piModel";

export interface DraftModelRow {
  key: string;
  model: ModelEntry;
}

export interface ProfileDraftState {
  value: ProviderProfile;
  rows: Record<string, DraftModelRow[]>;
  rawText: string;
  rawError: string | null;
  dirty: boolean;
}

export type ProfileDraftAction =
  | { type: "setProfileField"; field: string; value: unknown }
  | { type: "setRawText"; text: string }
  | { type: "setChannelModels"; channel: string; models: ModelEntry[] }
  | { type: "appendModel"; channel: string; key: string; model: ModelEntry }
  | { type: "setModel"; channel: string; key: string; model: ModelEntry }
  | { type: "setExposed"; channel: string; id: string; exposed: boolean }
  | { type: "setExposedSet"; channel: string; ids: string[] }
  | { type: "removeModel"; channel: string; key: string }
  | { type: "resetFromServer"; profile: ProviderProfile };

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function rowKey(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return Math.random().toString(36).slice(2);
}

function modelList(profile: ProviderProfile, channel: string): ModelEntry[] {
  const upstream = (profile.upstreams ?? []).find((item) => (item.name ?? "").trim() === channel);
  if (upstream) return upstream.models ?? [];
  if (channel === "main") {
    const legacy = profile as ProviderProfile & { models?: ModelEntry[] };
    return legacy.models ?? [];
  }
  return [];
}

export function channelNames(profile: ProviderProfile): string[] {
  const upstreams = profile.upstreams ?? [];
  if (upstreams.length === 0 || (upstreams.length === 1 && !(upstreams[0].name ?? "").trim())) return ["main"];
  return upstreams.map((item) => (item.name ?? "").trim()).filter(Boolean);
}

export function materializeMainChannel(profile: ProviderProfile): ProviderProfile {
  const cloned = clone(profile);
  const upstreams = cloned.upstreams ?? [];
  if (upstreams.length > 0 && upstreams.some((item) => (item.name ?? "").trim())) return cloned;
  const legacy = cloned as ProviderProfile & { models?: ModelEntry[]; exposedModels?: string[] };
  const source = upstreams[0];
  const main: Upstream = {
    ...(source ?? {}),
    name: "main",
    api: source?.api ?? cloned.api,
    responsesMode: source?.responsesMode ?? cloned.responsesMode ?? "auto",
    baseUrl: source?.baseUrl ?? cloned.baseUrl,
    apiKey: source?.apiKey ?? cloned.apiKey,
    headers: source?.headers ?? cloned.headers,
    models: source?.models ?? legacy.models ?? [],
    exposedModels: source?.exposedModels ?? legacy.exposedModels ?? [],
  };
  return { ...cloned, responsesMode: cloned.responsesMode ?? "auto", upstreams: [main] };
}

function reconcileRows(previous: DraftModelRow[] | undefined, models: ModelEntry[]): DraftModelRow[] {
  return models.map((model, index) => ({
    key: previous?.[index]?.key ?? previous?.find((row) => row.model.id === model.id)?.key ?? rowKey(),
    model: clone(model),
  }));
}

function rowsFromProfile(profile: ProviderProfile, previous: Record<string, DraftModelRow[]> = {}): Record<string, DraftModelRow[]> {
  const rows: Record<string, DraftModelRow[]> = {};
  for (const channel of channelNames(profile)) rows[channel] = reconcileRows(previous[channel], modelList(profile, channel));
  return rows;
}

function serialized(profile: ProviderProfile): string {
  return JSON.stringify(profile, null, 2);
}

function initialValue(profile: ProviderProfile): ProviderProfile {
  const value = clone(profile);
  if (!value.responsesMode) value.responsesMode = "auto";
  return value;
}

export function createProfileDraft(profile: ProviderProfile): ProfileDraftState {
  const value = initialValue(profile);
  return {
    value,
    rows: rowsFromProfile(value),
    rawText: serialized(value),
    rawError: null,
    dirty: false,
  };
}

function updateValue(state: ProfileDraftState, value: ProviderProfile, rows = state.rows): ProfileDraftState {
  const next = initialValue(value);
  return {
    value: next,
    rows: rowsFromProfile(next, rows),
    rawText: serialized(next),
    rawError: null,
    dirty: true,
  };
}

function updateChannel(
  profile: ProviderProfile,
  channel: string,
  update: (upstream: Upstream) => Upstream,
): ProviderProfile {
  const next = clone(profile);
  const upstreams = [...(next.upstreams ?? [])];
  const index = upstreams.findIndex((item) => (item.name ?? "").trim() === channel);
  if (index >= 0) {
    upstreams[index] = update(upstreams[index]);
  } else {
    const legacy = next as ProviderProfile & { models?: ModelEntry[]; exposedModels?: string[] };
    const base: Upstream = {
      name: channel,
      api: next.api,
      responsesMode: next.responsesMode ?? "auto",
      baseUrl: next.baseUrl,
      apiKey: next.apiKey,
      headers: next.headers,
      models: channel === "main" ? legacy.models ?? [] : [],
      exposedModels: channel === "main" ? legacy.exposedModels ?? [] : [],
    };
    upstreams.push(update(base));
  }
  return { ...next, upstreams };
}

function setChannelModels(state: ProfileDraftState, channel: string, models: ModelEntry[]): ProfileDraftState {
  const ids = new Set(models.map((model) => model.id));
  return updateValue(
    state,
    updateChannel(state.value, channel, (upstream) => ({
      ...upstream,
      models: clone(models),
      exposedModels: (upstream.exposedModels ?? []).filter((id) => ids.has(id)),
    })),
  );
}

export function profileDraftReducer(state: ProfileDraftState, action: ProfileDraftAction): ProfileDraftState {
  switch (action.type) {
    case "setProfileField":
      return updateValue(state, { ...state.value, [action.field]: action.value });
    case "setRawText": {
      const result = validateProfileJson(action.text);
      if (!result.ok || !result.value) {
        return { ...state, rawText: action.text, rawError: result.error ?? "Invalid profile JSON", dirty: true };
      }
      const next = updateValue(state, result.value as unknown as ProviderProfile);
      return { ...next, rawText: action.text };
    }
    case "setChannelModels":
      return setChannelModels(state, action.channel, action.models);
    case "appendModel": {
      const next = updateValue(
        state,
        updateChannel(state.value, action.channel, (upstream) => ({
          ...upstream,
          models: [...(upstream.models ?? []), clone(action.model)],
        })),
        state.rows,
      );
      next.rows = {
        ...next.rows,
        [action.channel]: [...(state.rows[action.channel] ?? []), { key: action.key, model: clone(action.model) }],
      };
      return next;
    }
    case "setModel": {
      const rows = state.rows[action.channel] ?? [];
      const index = rows.findIndex((row) => row.key === action.key);
      if (index < 0) return state;
      const old = rows[index].model;
      let nextProfile = updateChannel(state.value, action.channel, (upstream) => {
        const models = [...(upstream.models ?? [])];
        models[index] = clone(action.model);
        let exposed = [...(upstream.exposedModels ?? [])];
        if (old.id !== action.model.id && exposed.includes(old.id)) {
          exposed = exposed.filter((id) => id !== old.id);
          if (action.model.id.trim()) exposed.push(action.model.id);
        }
        return { ...upstream, models, exposedModels: exposed };
      });
      return updateValue(state, nextProfile, state.rows);
    }
    case "setExposed":
      return updateValue(
        state,
        updateChannel(state.value, action.channel, (upstream) => {
          const id = action.id.trim();
          if (!id) return upstream;
          const exposed = new Set(upstream.exposedModels ?? []);
          if (action.exposed) exposed.add(id);
          else exposed.delete(id);
          return { ...upstream, exposedModels: [...exposed] };
        }),
        state.rows,
      );
    case "setExposedSet":
      return updateValue(
        state,
        updateChannel(state.value, action.channel, (upstream) => ({
          ...upstream,
          exposedModels: [...new Set(action.ids.map((id) => id.trim()).filter(Boolean))],
        })),
        state.rows,
      );
    case "removeModel": {
      const rows = state.rows[action.channel] ?? [];
      const index = rows.findIndex((row) => row.key === action.key);
      if (index < 0) return state;
      const removedId = rows[index].model.id;
      return updateValue(
        state,
        updateChannel(state.value, action.channel, (upstream) => ({
          ...upstream,
          models: (upstream.models ?? []).filter((_, itemIndex) => itemIndex !== index),
          exposedModels: (upstream.exposedModels ?? []).filter((id) => id !== removedId),
        })),
        state.rows,
      );
    }
    case "resetFromServer":
      return createProfileDraft(action.profile);
  }
}

export function useProfileDraft(profile: ProviderProfile) {
  const [state, dispatch] = useReducer(profileDraftReducer, profile, createProfileDraft);
  const resetFromServer = useCallback((next: ProviderProfile) => {
    dispatch({ type: "resetFromServer", profile: next });
  }, []);
  return { state, dispatch, resetFromServer };
}

export function parseModelsDraft(text: string): { ok: true; models: ModelEntry[] } | { ok: false; error: string } {
  const result = validateModelsJson(text);
  if (!result.ok || !result.value) return { ok: false, error: result.error ?? "Invalid models JSON" };
  return { ok: true, models: result.value as unknown as ModelEntry[] };
}

export function modelsForChannel(profile: ProviderProfile, channel: string): ModelEntry[] {
  return modelList(profile, channel);
}

export function exposedForChannel(profile: ProviderProfile, channel: string): Set<string> {
  const upstream = (profile.upstreams ?? []).find((item) => (item.name ?? "").trim() === channel);
  return new Set(upstream?.exposedModels ?? []);
}

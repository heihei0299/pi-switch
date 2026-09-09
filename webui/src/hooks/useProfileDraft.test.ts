import { describe, expect, it } from "vitest";
import type { ProviderProfile } from "../types";
import { entryFromDraft, newModelDraft } from "../lib/piModel";
import {
  createProfileDraft,
  materializeMainChannel,
  profileDraftReducer,
} from "./useProfileDraft";

function profile(): ProviderProfile {
  return {
    api: "openai-completions",
    responsesMode: "auto",
    baseUrl: "https://example.test/v1",
    apiKey: "key",
    proxy: false,
    upstreams: [{
      name: "main",
      api: "openai-completions",
      responsesMode: "auto",
      baseUrl: "https://example.test/v1",
      apiKey: "key",
      models: [{ id: "old", name: "Old" } as any],
      exposedModels: ["old"],
    }],
  };
}

describe("profile draft reducer", () => {
  it("keeps row identity and moves exposure when a model id changes", () => {
    let state = createProfileDraft(profile());
    const key = state.rows.main[0].key;
    state = profileDraftReducer(state, {
      type: "setModel",
      channel: "main",
      key,
      model: { ...state.rows.main[0].model, id: "new" },
    });

    expect(state.rows.main[0].key).toBe(key);
    expect(state.value.upstreams?.[0].models?.[0].id).toBe("new");
    expect(state.value.upstreams?.[0].exposedModels).toEqual(["new"]);
  });

  it("replaces the canonical value on valid raw input but preserves it on invalid input", () => {
    let state = createProfileDraft(profile());
    const raw = JSON.stringify({ ...profile(), baseUrl: "https://changed.example/v1" });
    state = profileDraftReducer(state, { type: "setRawText", text: raw });
    expect(state.value.baseUrl).toBe("https://changed.example/v1");
    expect(state.rawError).toBeNull();

    const previous = state.value;
    state = profileDraftReducer(state, { type: "setRawText", text: "{ broken" });
    expect(state.value).toEqual(previous);
    expect(state.rawText).toBe("{ broken");
    expect(state.rawError).toMatch(/Invalid JSON/);
  });

  it("synchronizes exposure when a valid model list is replaced", () => {
    let state = createProfileDraft(profile());
    state = profileDraftReducer(state, {
      type: "setChannelModels",
      channel: "main",
      models: [{ id: "replacement" } as any],
    });
    expect(state.value.upstreams?.[0].exposedModels).toEqual([]);
  });

  it("removes a model and its exposure from the same canonical channel value", () => {
    let state = createProfileDraft(profile());
    state = profileDraftReducer(state, {
      type: "removeModel",
      channel: "main",
      key: state.rows.main[0].key,
    });

    expect(state.value.upstreams?.[0].models).toEqual([]);
    expect(state.value.upstreams?.[0].exposedModels).toEqual([]);
  });

  it("materializes legacy profiles without fabricating model metadata", () => {
    const legacy = { ...profile(), upstreams: undefined } as ProviderProfile & { models?: unknown[] };
    legacy.models = [{ id: "legacy" }];
    const materialized = materializeMainChannel(legacy);
    expect(materialized.upstreams?.[0].name).toBe("main");
    expect(materialized.upstreams?.[0].models).toEqual([{ id: "legacy" }]);
    expect(materialized.upstreams?.[0].models?.[0]).not.toHaveProperty("contextWindow");
    expect(materialized.upstreams?.[0].models?.[0]).not.toHaveProperty("maxTokens");
  });

  it("preserves the explicit key when appending a model", () => {
    let state = createProfileDraft(profile());
    const draft = newModelDraft();
    draft.id = "added";
    state = profileDraftReducer(state, {
      type: "appendModel",
      channel: "main",
      key: draft.key,
      model: entryFromDraft(draft),
    });
    expect(state.rows.main[state.rows.main.length - 1]?.key).toBe(draft.key);
    const models = state.value.upstreams?.[0].models ?? [];
    expect(models[models.length - 1]?.id).toBe("added");
  });
});

import { describe, expect, it } from "vitest";
import { validateModelEntry, validateModelsJson, validateProfileJson } from "./piModel";

describe("piModel validation — ModelEntry 1:1 with pi models.json", () => {
  describe("validateModelEntry", () => {
    it("rejects missing id", () => {
      const res = validateModelEntry({ name: "foo" } as any);
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/id/);
    });
    it("rejects empty id", () => {
      const res = validateModelEntry({ id: "   " } as any);
      expect(res.ok).toBe(false);
    });
    it("accepts minimal valid entry", () => {
      const res = validateModelEntry({ id: "m1" } as any);
      expect(res.ok).toBe(true);
      expect(res.value?.id).toBe("m1");
    });
    it("rejects non-positive contextWindow", () => {
      const res = validateModelEntry({ id: "m1", contextWindow: -1 } as any);
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/contextWindow/);
    });
    it("rejects non-positive maxTokens", () => {
      const res = validateModelEntry({ id: "m1", maxTokens: 0 } as any);
      expect(res.ok).toBe(false);
    });
    it("accepts cost with tiers", () => {
      const res = validateModelEntry({ id: "m1", cost: { input: 1, output: 2, cacheRead: 0, cacheWrite: 0, tiers: [{ inputTokensAbove: 100, input: 2, output: 3, cacheRead: 0, cacheWrite: 0 }] } } as any);
      expect(res.ok).toBe(true);
    });
    it("rejects invalid cost shape", () => {
      const res = validateModelEntry({ id: "m1", cost: { input: "bad" } } as any);
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/cost/);
    });
    it("accepts legacy context_window and normalizes to camelCase", () => {
      const res = validateModelEntry({ id: "m1", context_window: 123 } as any);
      expect(res.ok).toBe(true);
      expect(res.value?.contextWindow).toBe(123);
      expect((res.value as any)?.context_window).toBeUndefined();
    });
    it("preserves passthrough unknown fields", () => {
      const res = validateModelEntry({ id: "m1", customField: "keep", headers: { "x-a": "1" } } as any);
      expect(res.ok).toBe(true);
      expect((res.value as any).customField).toBe("keep");
    });
  });

  describe("validateModelsJson", () => {
    it("rejects invalid JSON", () => {
      const res = validateModelsJson("{ broken");
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/JSON/);
    });
    it("rejects non-array", () => {
      const res = validateModelsJson(JSON.stringify({ id: "m1" }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/array/);
    });
    it("rejects array with invalid entry", () => {
      const res = validateModelsJson(JSON.stringify([{ id: "" }]));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/id/);
    });
    it("accepts valid array and allows passthrough", () => {
      const arr = [{ id: "m1", name: "M1", cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, extra: "keep" }];
      const res = validateModelsJson(JSON.stringify(arr));
      expect(res.ok).toBe(true);
      expect(res.value).toHaveLength(1);
      expect((res.value?.[0] as any).extra).toBe("keep");
    });
    it("normalizes legacy keys in array", () => {
      const res = validateModelsJson(JSON.stringify([{ id: "m1", context_window: 200 }]));
      expect(res.ok).toBe(true);
      expect((res.value?.[0] as any).contextWindow).toBe(200);
    });
    it("rejects duplicate ids", () => {
      const res = validateModelsJson(JSON.stringify([{ id: "m1" }, { id: "m1" }]));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/duplicate/i);
    });
  });

  describe("validateProfileJson — 1:1 provider shape (api/baseUrl)", () => {
    it("rejects invalid JSON", () => {
      const res = validateProfileJson("{");
      expect(res.ok).toBe(false);
    });
    it("rejects unsupported api", () => {
      const res = validateProfileJson(JSON.stringify({ api: "bad", baseUrl: "http://a/v1" }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/api/);
    });
    it("rejects bad baseUrl", () => {
      const res = validateProfileJson(JSON.stringify({ api: "openai-completions", baseUrl: "not-url" }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/baseUrl/);
    });
    it("accepts valid profile", () => {
      const res = validateProfileJson(JSON.stringify({ api: "openai-completions", baseUrl: "https://api.example.com/v1" }));
      expect(res.ok).toBe(true);
    });
    it("accepts upstreams variant", () => {
      const res = validateProfileJson(JSON.stringify({ api: "openai-completions", baseUrl: "https://a/v1", upstreams: [{ baseUrl: "https://b/v1", apiKey: "k" }] }));
      expect(res.ok).toBe(true);
    });
  });
});

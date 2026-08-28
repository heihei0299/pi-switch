import { describe, expect, it } from "vitest";
import { detectConflicts, diffGateway, validateGatewayJson } from "./gatewayDiff";

describe("gatewayDiff", () => {
  describe("diffGateway", () => {
    it("detects added/removed/changed top-level keys", () => {
      const current = { api: "openai-completions", baseUrl: "http://a/v1", models: [], proxy: false, headers: { "x-a": "1" } };
      const proposed = { api: "openai-completions", baseUrl: "http://b/v1", models: [], proxy: false, extra: 1 };
      const diff = diffGateway(current, proposed);
      expect(diff.changed).toContain("baseUrl");
      expect(diff.added).toContain("extra");
      expect(diff.removed).toContain("headers");
    });

    it("handles missing current as all added", () => {
      const proposed = { api: "openai-completions", baseUrl: "http://b/v1", models: [], proxy: false };
      const diff = diffGateway(null, proposed);
      expect(diff.added).toEqual(expect.arrayContaining(["api", "baseUrl", "models", "proxy"]));
      expect(diff.removed).toEqual([]);
    });

    it("detects model count change as models changed", () => {
      const cur = { models: [{ id: "a/b" }], api: "openai-completions", baseUrl: "http://a/v1", proxy: false };
      const prop = { models: [{ id: "a/b" }, { id: "a/c" }], api: "openai-completions", baseUrl: "http://a/v1", proxy: false };
      const diff = diffGateway(cur, prop);
      expect(diff.changed).toContain("models");
    });
  });

  describe("detectConflicts", () => {
    it("returns keys from preview conflicts that are changed", () => {
      const current = { api: "openai-completions", baseUrl: "http://a/v1", models: [] };
      const proposed = { api: "openai-completions", baseUrl: "http://b/v1", models: [] };
      const conflicts = ["baseUrl"];
      expect(detectConflicts(current, proposed, conflicts)).toEqual(["baseUrl"]);
    });

    it("filters out non-conflicting keys", () => {
      const current = { api: "openai-completions", baseUrl: "http://a/v1", models: [] };
      const proposed = { api: "openai-completions", baseUrl: "http://a/v1", models: [] };
      expect(detectConflicts(current, proposed, ["baseUrl"])).toEqual([]);
    });
  });

  describe("validateGatewayJson", () => {
    it("rejects invalid JSON", () => {
      const res = validateGatewayJson("{ broken");
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/JSON/);
    });

    it("rejects missing required fields", () => {
      const res = validateGatewayJson(JSON.stringify({ api: "openai-completions" }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/baseUrl/);
    });

    it("rejects invalid api", () => {
      const res = validateGatewayJson(JSON.stringify({ api: "invalid", baseUrl: "http://a/v1", models: [] }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/api/);
    });

    it("rejects invalid baseUrl", () => {
      const res = validateGatewayJson(JSON.stringify({ api: "openai-completions", baseUrl: "not-a-url", models: [] }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/baseUrl/);
    });

    it("rejects models not array", () => {
      const res = validateGatewayJson(JSON.stringify({ api: "openai-completions", baseUrl: "http://a/v1", models: "bad" }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/models/);
    });

    it("accepts valid gateway", () => {
      const valid = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", apiKey: "x", models: [{ id: "p/m" }], proxy: false };
      const res = validateGatewayJson(JSON.stringify(valid));
      expect(res.ok).toBe(true);
      expect(res.value).toEqual(valid);
    });

    it("rejects model without id", () => {
      const valid = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ noId: 1 }] };
      const res = validateGatewayJson(JSON.stringify(valid));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/id/);
    });
  });
});

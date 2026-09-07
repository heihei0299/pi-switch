import { describe, expect, it } from "vitest";
import { detectConflicts, diffGateway, validateGatewayJson } from "./gatewayDiff";

describe("gatewayDiff", () => {
  describe("diffGateway", () => {
    it("detects added/removed/changed provider entries", () => {
      const current = { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: [], proxy: false, headers: { "x-a": "1" } } };
      const proposed = { "sup/chat": { api: "openai-completions", baseUrl: "http://b/v1", models: [], proxy: false, extra: 1 }, "sup/new": { api: "openai-completions", baseUrl: "http://a/v1", models: [] } };
      const diff = diffGateway(current, proposed);
      expect(diff.changed).toContain("sup/chat");
      expect(diff.added).toContain("sup/new");
      expect(diff.removed).toEqual([]);
    });

    it("handles missing current as all added", () => {
      const proposed = { "sup/chat": { api: "openai-completions", baseUrl: "http://b/v1", models: [], proxy: false } };
      const diff = diffGateway(null, proposed);
      expect(diff.added).toEqual(["sup/chat"]);
      expect(diff.removed).toEqual([]);
    });

    it("detects model count change as models changed", () => {
      const cur = { "sup/chat": { models: [{ id: "m1" }], api: "openai-completions", baseUrl: "http://a/v1", proxy: false } };
      const prop = { "sup/chat": { models: [{ id: "m1" }, { id: "m2" }], api: "openai-completions", baseUrl: "http://a/v1", proxy: false } };
      const diff = diffGateway(cur, prop);
      expect(diff.added).toContain("sup/chat/m2");
    });

    it("compares bare model ids within their provider", () => {
      const cur = {
        "sup/chat": { api: "openai-completions", baseUrl: "http://chat/v1", models: [{ id: "m1" }] },
        "sup/responses": { api: "openai-responses", baseUrl: "http://responses/v1", models: [{ id: "m1" }] },
      };
      const prop = {
        "sup/chat": { api: "openai-completions", baseUrl: "http://chat/v1", models: [{ id: "m1" }, { id: "m2" }] },
        "sup/responses": { api: "openai-responses", baseUrl: "http://responses/v1", models: [{ id: "m1" }] },
      };
      const diff = diffGateway(cur, prop);
      expect(diff.added).toEqual(["sup/chat/m2"]);
      expect(diff.removed).toEqual([]);
      expect(diff.changed).toEqual([]);
    });
  });

  describe("detectConflicts", () => {
    it("returns keys from preview conflicts that are changed", () => {
      const current = { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: [] } };
      const proposed = { "sup/chat": { api: "openai-completions", baseUrl: "http://b/v1", models: [] } };
      const conflicts = ["sup/chat"];
      expect(detectConflicts(current, proposed, conflicts)).toEqual(["sup/chat"]);
    });

    it("filters out non-conflicting keys", () => {
      const current = { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: [] } };
      const proposed = { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: [] } };
      expect(detectConflicts(current, proposed, ["sup/chat"])).toEqual([]);
    });
  });

  describe("validateGatewayJson", () => {
    it("rejects invalid JSON", () => {
      const res = validateGatewayJson("{ broken");
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/JSON/);
    });

    it("rejects a missing providers wrapper", () => {
      const res = validateGatewayJson(JSON.stringify({}));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/providers/);
    });

    it("rejects invalid api", () => {
      const res = validateGatewayJson(JSON.stringify({ providers: { "sup/chat": { api: "invalid", baseUrl: "http://a/v1", models: [] } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/api/);
    });

    it("rejects invalid baseUrl", () => {
      const res = validateGatewayJson(JSON.stringify({ providers: { "sup/chat": { api: "openai-completions", baseUrl: "not-a-url", models: [] } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/baseUrl/);
    });

    it("rejects models not array", () => {
      const res = validateGatewayJson(JSON.stringify({ providers: { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: "bad" } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/models/);
    });

    it("accepts valid gateway", () => {
      const valid = { providers: { "sup/chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", apiKey: "x", models: [{ id: "m" }], proxy: false } } };
      const res = validateGatewayJson(JSON.stringify(valid));
      expect(res.ok).toBe(true);
      expect(res.value).toEqual(valid);
    });

    it("rejects model without id", () => {
      const valid = { providers: { "sup/chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ noId: 1 }] } } };
      const res = validateGatewayJson(JSON.stringify(valid));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/id/);
    });

    it("accepts a providers wrapper with bare ids", () => {
      const valid = {
        providers: {
          "sup/chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: [{ id: "m1" }],
          },
        },
      };
      expect(validateGatewayJson(JSON.stringify(valid))).toEqual({ ok: true, value: valid });
    });

    it("rejects slash in a provider model id", () => {
      const invalid = {
        providers: {
          "sup/chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: [{ id: "sup/chat/m1" }],
          },
        },
      };
      const res = validateGatewayJson(JSON.stringify(invalid));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/must not contain/);
    });
  });
});

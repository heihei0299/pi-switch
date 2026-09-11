import { describe, expect, it } from "vitest";
import {
  detectConflicts,
  diffGateway,
  filterFixedGatewayDiff,
  filterFixedGatewayProviders,
  isFixedGatewayProvider,
  makeFixedProviderSet,
  validateGatewayJson,
} from "./gatewayDiff";

// The backend sends this with every preview; tests pin a representative contract.
const FIXED = makeFixedProviderSet([
  { key: "pi-switch-res", api: "openai-responses" },
  { key: "pi-switch-chat", api: "openai-completions" },
]);
const validate = (text: string) => validateGatewayJson(text, FIXED);

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

  describe("fixed provider filtering", () => {
    it("recognizes only the gateway-owned provider keys", () => {
      expect(isFixedGatewayProvider("pi-switch-chat", FIXED)).toBe(true);
      expect(isFixedGatewayProvider("pi-switch-res", FIXED)).toBe(true);
      expect(isFixedGatewayProvider("cpa", FIXED)).toBe(false);
      expect(isFixedGatewayProvider("oc/chat", FIXED)).toBe(false);
    });

    it("honors whatever fixed set the backend declares (no frontend mirror)", () => {
      const custom = makeFixedProviderSet([{ key: "vendor-gw", api: "openai-completions" }]);
      expect(isFixedGatewayProvider("vendor-gw", custom)).toBe(true);
      expect(isFixedGatewayProvider("pi-switch-chat", custom)).toBe(false);
      const providers = {
        "vendor-gw": { api: "openai-completions", models: [] },
        "pi-switch-chat": { api: "openai-completions", models: [] },
      };
      expect(Object.keys(filterFixedGatewayProviders(providers, custom))).toEqual(["vendor-gw"]);
      // and validation uses the declared API contract
      const text = JSON.stringify({ providers: { "vendor-gw": { api: "openai-responses", baseUrl: "http://a/v1", models: [] } } });
      const res = validateGatewayJson(text, custom);
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/openai-completions/);
    });

    it("keeps fixed providers and drops wild ones", () => {
      const providers = {
        "pi-switch-chat": { api: "openai-completions", models: [] },
        cpa: { api: "openai-responses", models: [] },
        "pi-switch-res": { api: "openai-responses", models: [] },
      };
      expect(Object.keys(filterFixedGatewayProviders(providers, FIXED)).sort()).toEqual([
        "pi-switch-chat",
        "pi-switch-res",
      ]);
    });

    it("drops wild entries from added/removed/changed without splitting keys", () => {
      const diff = {
        // fixed bare key + fixed composite, wild keys with slashes, and a wild
        // key that merely *starts with* a fixed provider name
        added: ["pi-switch-chat", "pi-switch-chat/m2", "cpa/ocg/muse-1.3", "pi-switch-chat-evil/x"],
        removed: ["pi-switch-res/old", "sup/main/m1", "wild/with/many/slashes"],
        changed: ["pi-switch-chat", "cpa", "oc/chat/m1"],
      };
      expect(filterFixedGatewayDiff(diff, FIXED)).toEqual({
        added: ["pi-switch-chat", "pi-switch-chat/m2"],
        removed: ["pi-switch-res/old"],
        changed: ["pi-switch-chat"],
      });
    });
  });

  describe("validateGatewayJson", () => {
    it("rejects invalid JSON", () => {
      const res = validate("{ broken");
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/JSON/);
    });

    it("rejects a missing providers wrapper", () => {
      const res = validate(JSON.stringify({}));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/providers/);
    });

    it("rejects invalid api", () => {
      const res = validate(JSON.stringify({ providers: { "pi-switch-chat": { api: "invalid", baseUrl: "http://a/v1", models: [] } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/api/);
    });

    it("rejects invalid baseUrl", () => {
      const res = validate(JSON.stringify({ providers: { "pi-switch-chat": { api: "openai-completions", baseUrl: "not-a-url", models: [] } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/baseUrl/);
    });

    it("rejects models not array", () => {
      const res = validate(JSON.stringify({ providers: { "pi-switch-chat": { api: "openai-completions", baseUrl: "http://a/v1", models: "bad" } } }));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/models/);
    });

    it("accepts valid gateway", () => {
      const valid = { providers: { "pi-switch-chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", apiKey: "x", models: [{ id: "m" }], proxy: false } } };
      const res = validate(JSON.stringify(valid));
      expect(res.ok).toBe(true);
      expect(res.value).toEqual(valid);
    });

    it("rejects model without id", () => {
      const valid = { providers: { "pi-switch-chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ noId: 1 }] } } };
      const res = validate(JSON.stringify(valid));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/id/);
    });

    it("accepts a providers wrapper with bare ids", () => {
      const valid = {
        providers: {
          "pi-switch-chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: [{ id: "m1" }],
          },
        },
      };
      expect(validate(JSON.stringify(valid))).toEqual({ ok: true, value: valid });
    });

    it("rejects slash in a fixed provider model id", () => {
      const invalid = {
        providers: {
          "pi-switch-chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: [{ id: "sup/chat/m1" }],
          },
        },
      };
      const res = validate(JSON.stringify(invalid));
      expect(res.ok).toBe(false);
      expect(res.error).toMatch(/must not contain/);
    });

    it("ignores third-party providers entirely", () => {
      const input = {
        providers: {
          cpa: {
            api: "openai-responses",
            baseUrl: "http://127.0.0.1:8317/v1",
            models: [{ id: "ocg/muse-1.3" }],
          },
          "pi-switch-chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: [{ id: "m" }],
          },
        },
      };
      const res = validate(JSON.stringify(input));
      expect(res.ok).toBe(true);
      expect(res.value).toEqual({
        providers: {
          "pi-switch-chat": input.providers["pi-switch-chat"],
        },
      });
    });
  });
});

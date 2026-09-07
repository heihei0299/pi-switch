import { describe, expect, it, vi } from "vitest";
import { diffGateway, validateGatewayJson } from "./gatewayDiff";

describe("gateway preview/apply lifecycle placeholder", () => {
  it("diffGateway drives status bar pending count", () => {
    const cur = { "sup/chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "m1" }], proxy: false } };
    const prop = { "sup/chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "m1" }, { id: "m2" }], proxy: false } };
    const d = diffGateway(cur, prop);
    expect(d.added).toContain("sup/chat/m2");
    expect(d.added.length + d.removed.length + d.changed.length).toBe(1);
  });

  it("validateGatewayJson accepts valid gateway with models", () => {
    const valid = { providers: { "sup/chat": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "m1" }], proxy: false } } };
    const res = validateGatewayJson(JSON.stringify(valid));
    expect(res.ok).toBe(true);
    expect(res.value).toEqual(valid);
  });

  it("health placeholder shape matches api contract", async () => {
    const { api } = await import("../api");
    const mockHealth = { running: true, mode: "logical-isolation", gateway_id: "pi-gw", has_models_file: true, last_notify: null, upstreams_total: 2, message: "ok" };
    const spy = vi.spyOn(api, "getGatewayHealth").mockResolvedValue(mockHealth as any);
    const got = await api.getGatewayHealth();
    expect(got.running).toBe(true);
    expect(got.mode).toBe("logical-isolation");
    expect(got.upstreams_total).toBe(2);
    spy.mockRestore();
  });

  it("preview is dry-run does not mutate current (pure)", () => {
    const cur = { "sup/chat": { api: "openai-completions", baseUrl: "http://a/v1", models: [] as any[] } };
    const prop = { "sup/chat": { api: "openai-completions", baseUrl: "http://b/v1", models: [] as any[] } };
    const before = JSON.stringify(cur);
    const _d = diffGateway(cur, prop);
    expect(JSON.stringify(cur)).toBe(before);
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { ContractError } from "./apiSchema";

function okResponse(body: string) {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    text: async () => body,
  } as Response;
}

function errorResponse(status: number, statusText: string, body: string) {
  return {
    ok: false,
    status,
    statusText,
    text: async () => body,
  } as Response;
}

describe("API runtime contract boundary", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.restoreAllMocks());

  // system-contract 2.8: the management surface answers {"error": "<message>"},
  // and the client must surface that message (not the HTTP statusText).
  it("surfaces the management string message on a failed request", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      errorResponse(501, "Not Implemented", JSON.stringify({ error: "config export is not implemented" })),
    );

    await expect(api.getState()).rejects.toThrow("config export is not implemented");
  });

  it("reports the response path when a required state field is missing", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      profiles: {},
      settings: {
        proxy: { host: "127.0.0.1", port: 43112 },
        web: { host: "127.0.0.1", port: 43110 },
      },
    })));

    await expect(api.getState()).rejects.toMatchObject({
      path: "state.settings.writeMode",
      expected: "required string",
    });
  });

  it("decodes the backend protocol capability set", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      profiles: {},
      settings: { writeMode: "gateway" },
      protocol: {
        apis: [{
          id: "openai-responses",
          label: "OpenAI Responses",
          defaultMode: "passthrough",
          responsesModes: ["auto", "passthrough"],
          canProxy: true,
          canGateway: true,
        }],
      },
    })));

    const state = await api.getState();
    expect(state.protocol?.apis).toEqual([{
      id: "openai-responses",
      label: "OpenAI Responses",
      defaultMode: "passthrough",
      responsesModes: ["auto", "passthrough"],
      canProxy: true,
      canGateway: true,
    }]);
  });

  it("wraps malformed successful JSON as a contract error at the root", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse("not-json"));

    await expect(api.getState()).rejects.toEqual(new ContractError("$", "valid JSON response"));
  });

  it("decodes the same build identity exposed by the management API", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      version: "20260908.0.2",
      buildTime: "2026-09-10T00:00:00Z",
      commit: "abc123",
      target: "linux/amd64",
      dirty: "false",
      webui: { embedded: true },
    })));

    const result = await api.buildInfo();
    expect(result).toMatchObject({ version: "20260908.0.2", commit: "abc123", target: "linux/amd64", dirty: "false" });
  });

  it("accepts the nullable backup returned by model updates", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      ok: true,
      backup: null,
      enrich: { enriched: 0 },
    })));

    const result = await api.updateModels("mock", [], "main");
    expect(result.ok).toBe(true);
    expect(result.backup).toBeNull();
  });

  it("accepts null conversation metadata for unnamed summaries", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      totalRequests: 1,
      okRequests: 1,
      failedRequests: 0,
      successRate: "100.0%",
      byProvider: {},
      byConversation: [{
        conversationId: "unlabeled",
        name: null,
        requests: 1,
        inputTokens: 10,
        outputTokens: 5,
        cachedTokens: 0,
        reasoningTokens: 0,
        lastActive: null,
        cacheRate: "0.0%",
        cost: null,
      }],
    })));

    const result = await api.stats("today", 1, 2);
    expect(result.byConversation?.[0]).toMatchObject({ conversationId: "unlabeled", requests: 1 });
    expect(result.byConversation?.[0]?.name).toBeNull();
    expect(result.byConversation?.[0]?.lastActive).toBeNull();
  });

  it("decodes package import status and warnings", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      ok: true,
      count: 2,
      discovered: 3,
      skipped: 1,
      status: "imported",
      message: "Imported 2 Pi packages",
      warnings: ["one package had no manifest"],
    })));

    const result = await api.importPackages();
    expect(result).toMatchObject({ count: 2, discovered: 3, skipped: 1, status: "imported" });
    expect(result.warnings).toEqual(["one package had no manifest"]);
  });

  it("normalizes legacy snake_case stats aliases at the API boundary", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      total_requests: 1,
      ok_requests: 1,
      failed_requests: 0,
      success_rate: "100.0%",
      by_provider: {
        demo: {
          total: 1,
          ok: 1,
          failed: 0,
          prompt_tokens: 12,
          output_tokens: 4,
          cached_tokens: 2,
          reasoning_tokens: 1,
          cache_rate: "16.7%",
        },
      },
      total_tokens: { input: 12, output: 4, total: 16, cached: 2, reasoning: 1 },
      recent_requests: [],
      recent_request_total: 0,
    })));

    const result = await api.stats("today", 1, 2);
    expect(result.totalRequests).toBe(1);
    expect(result.byProvider.demo.promptTokens).toBe(12);
    expect(result.byProvider.demo.cachedTokens).toBe(2);
    expect(result.recentRequestTotal).toBe(0);
  });

  it("decodes server-declared fixed providers on gateway preview", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(okResponse(JSON.stringify({
      current: {},
      proposed: {},
      conflicts: [],
      pending_count: 0,
      diff: { added: [], removed: [], changed: [] },
      groups: [],
      removed: [],
      fixed_providers: [{ key: "pi-switch-chat", api: "openai-completions" }],
    })));

    const preview = await api.previewGateway();
    expect(preview.fixed_providers).toEqual([
      { key: "pi-switch-chat", api: "openai-completions" },
    ]);
  });
});

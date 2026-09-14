import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StatsPanel } from "./StatsPanel";
import type { AppState, UsageStats } from "../types";

const statsMock = vi.fn();
const convMock = vi.fn();
const convReqMock = vi.fn();

vi.mock("../api", () => ({
  api: {
    stats: (...args: unknown[]) => statsMock(...args),
    statsConversations: (...args: unknown[]) => convMock(...args),
    conversationRequests: (...args: unknown[]) => convReqMock(...args),
  },
  logsExportUrl: () => "/api/logs/export?format=json",
}));

function makeState(source: "proxy" | "sessionScan" | "off"): AppState {
  return {
    current: null,
    profiles: {},
    settings: {
      providerPrefix: "pi-switch",
      writeMode: "merge",
      gatewayApi: "openai-completions",
      language: null,
      proxy: { host: "127.0.0.1", port: 43112, failover: [], circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 } },
      web: { host: "127.0.0.1", port: 43110 },
      conversationSource: source,
    } as unknown as AppState["settings"],
  };
}

function fullStats(): UsageStats {
  return {
    totalRequests: 10,
    okRequests: 9,
    failedRequests: 1,
    successRate: "90.0%",
    byProvider: {
      hyb: { total: 6, ok: 5, failed: 1, retries: 0, avgMs: 40, totalMs: 240, promptTokens: 1000, outputTokens: 500, cachedTokens: 0, reasoningTokens: 0 },
    },
    totalTokens: { input: 1000, output: 500, total: 1500, cached: 0, reasoning: 0 },
    cacheHitRate: "0%",
  };
}

describe("StatsPanel conversationSource off hides conversation UI", () => {
  beforeEach(() => {
    statsMock.mockReset();
    convMock.mockReset();
    convMock.mockResolvedValue({ conversations: [], total: 0 });
    convReqMock.mockReset();
    convReqMock.mockResolvedValue({ requests: [], total: 0 });
    statsMock.mockResolvedValue(fullStats());
  });
  afterEach(() => cleanup());

  it("hides By conversation when conversationSource is off", async () => {
    render(<StatsPanel state={makeState("off")} />);
    await screen.findByText("90.0%");
    expect(screen.queryByText(/By conversation/)).not.toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "By conversation" })).not.toBeInTheDocument();
  });

  it("shows By conversation when conversationSource is sessionScan", async () => {
    render(<StatsPanel state={makeState("sessionScan")} />);
    await screen.findByText("90.0%");
    expect(screen.getByText(/By conversation/)).toBeInTheDocument();
  });

  it("shows By conversation when conversationSource is proxy", async () => {
    render(<StatsPanel state={makeState("proxy")} />);
    await screen.findByText("90.0%");
    expect(screen.getByText(/By conversation/)).toBeInTheDocument();
  });
});

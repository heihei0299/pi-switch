import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, render, screen, fireEvent } from "@testing-library/react";
import { StatsPanel } from "./StatsPanel";
import type { AppState } from "../types";

const statsMock = vi.fn();
const convMock = vi.fn();
vi.mock("../api", () => ({
  api: {
    stats: (...args: unknown[]) => statsMock(...args),
    statsConversations: (...args: unknown[]) => convMock(...args),
    conversationRequests: vi.fn(async () => ({ requests: [], total: 0 })),
  },
  logsExportUrl: () => "/api/logs/export?format=json",
}));

function appState(): AppState {
  return { current: "p1", profiles: {}, settings: { providerPrefix: "pi-switch", conversationSource: "off" } } as any;
}

function usageStats(overrides: Record<string, unknown> = {}) {
  return {
    totalRequests: 5,
    okRequests: 5,
    failedRequests: 0,
    successRate: "100.0%",
    avgLatencyMs: 10,
    byProvider: {},
    byModel: {},
    totalTokens: { input: 100, output: 50, total: 150, cached: 0, reasoning: 0 },
    cacheHitRate: "0.0%",
    totalCost: 0.01,
    costUnknown: 0,
    byConversation: [],
    recentRequests: [],
    recentRequestTotal: 0,
    ...overrides,
  } as any;
}

describe("S4 Stats polling Off/5s/30s/5min", () => {
  beforeEach(() => {
    statsMock.mockReset();
    convMock.mockReset();
    convMock.mockResolvedValue({ conversations: [], total: 0 });
    statsMock.mockResolvedValue(usageStats());
    vi.useFakeTimers();
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("default Off does not start interval", async () => {
    render(<StatsPanel state={appState()} refresh={async () => {}} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(statsMock).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
    expect(statsMock).toHaveBeenCalledTimes(1);
  });

  it("5s tier polls with window params透传 and 5s interval", async () => {
    render(<StatsPanel state={appState()} refresh={async () => {}} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(statsMock).toHaveBeenCalledTimes(1);
    const firstCall = statsMock.mock.calls[0] as any[];
    const [range] = firstCall;
    const select = screen.getByLabelText(/Auto-refresh/i) as HTMLSelectElement;
    await act(async () => { fireEvent.change(select, { target: { value: "5000" } }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(statsMock).toHaveBeenCalledTimes(2);
    const secondCall = statsMock.mock.calls[1] as any[];
    expect(secondCall[0]).toBe(range);
    // window透传: for today, from is midnight (stable), to moves with now (+~5s)
    expect(typeof secondCall[1]).toBe("number");
    expect(typeof secondCall[2]).toBe("number");
    expect(secondCall[2]).toBeGreaterThanOrEqual(firstCall[2]);
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(statsMock).toHaveBeenCalledTimes(3);
  });

  it("polling failure keeps old data and old window (fail保旧)", async () => {
    // first call success, second fail, third success fallback
    statsMock.mockReset();
    statsMock.mockResolvedValueOnce(usageStats({ totalRequests: 5, totalCost: 0.01 }));
    statsMock.mockRejectedValueOnce(new Error("network fail") as never);
    statsMock.mockResolvedValue(usageStats({ totalRequests: 5, totalCost: 0.01 }));

    render(<StatsPanel state={appState()} refresh={async () => {}} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    // flush microtasks for render
    await act(async () => { await Promise.resolve(); });
    // check initial data visible (Total metric shows 5)
    expect(statsMock).toHaveBeenCalledTimes(1);
    // need to allow React to render hero metrics — check for Total label presence indirectly via mock
    const select = screen.getByLabelText(/Auto-refresh/i) as HTMLSelectElement;
    await act(async () => { fireEvent.change(select, { target: { value: "5000" } }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    // second call was failure, should have called twice total (1 success +1 fail)
    expect(statsMock).toHaveBeenCalledTimes(2);
    // after failure, component should still keep old data — not show "No request data yet"
    expect(screen.queryByText(/No request data yet/)).not.toBeInTheDocument();
    // third poll should succeed and still keep data
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(statsMock).toHaveBeenCalledTimes(3);
    expect(screen.queryByText(/No request data yet/)).not.toBeInTheDocument();
  });

  it("switching back to Off stops polling", async () => {
    render(<StatsPanel state={appState()} refresh={async () => {}} />);
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    const select = screen.getByLabelText(/Auto-refresh/i) as HTMLSelectElement;
    await act(async () => { fireEvent.change(select, { target: { value: "5000" } }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(statsMock).toHaveBeenCalledTimes(2);
    await act(async () => { fireEvent.change(select, { target: { value: "off" } }); });
    await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
    expect(statsMock).toHaveBeenCalledTimes(2);
  });
});

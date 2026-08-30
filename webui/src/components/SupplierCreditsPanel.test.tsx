import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SupplierCreditsPanel } from "./SupplierCreditsPanel";
import { api } from "../api";
import type { ProviderProfile } from "../types";

function profile(overrides: Partial<ProviderProfile>): ProviderProfile {
  return {
    api: "openai-completions",
    baseUrl: "https://api.opencode.ai/v1",
    apiKey: "k",
    models: [],
    proxy: false,
    ...overrides,
  } as ProviderProfile;
}

const sampleCredits = {
  balance: 10,
  used: 40,
  total: 50,
  remaining: 10,
  percent: 80,
  resetAt: "2026-09-01T00:00:00Z",
  expiry: "2026-09-01T00:00:00Z",
  raw: {},
};

describe("SupplierCreditsPanel", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders credits fields, progress bar, refresh button and 主上游 label on success", async () => {
    vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    render(<SupplierCreditsPanel name="p1" profile={profile({})} />);
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    expect(screen.getByText(/已用/)).toBeInTheDocument();
    expect(screen.getByText(/总额/)).toBeInTheDocument();
    // check values present
    expect(screen.getByText(String(sampleCredits.balance))).toBeInTheDocument();
    expect(screen.getByText(String(sampleCredits.used))).toBeInTheDocument();
    expect(screen.getByText(String(sampleCredits.total))).toBeInTheDocument();
    expect(screen.getByText(/2026-09-01/)).toBeInTheDocument();
    // progress bar percent
    const bar = screen.getByTestId("credits-progress-bar");
    expect(bar.style.width).toBe("80%");
    // refresh button
    expect(screen.getByRole("button", { name: /刷新/ })).toBeInTheDocument();
    // 主上游
    expect(screen.getByText(/主上游/)).toBeInTheDocument();
  });

  it("fetches automatically on mount once", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    render(<SupplierCreditsPanel name="p1" profile={profile({})} />);
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
    expect(spy).toHaveBeenCalledWith("p1");
  });

  it("refresh button re-fetches and shows spinner while loading", async () => {
    let resolve: (v: any) => void = () => {};
    const spy = vi.spyOn(api, "getCredits").mockImplementation(() => new Promise((r) => (resolve = r)));
    render(<SupplierCreditsPanel name="p1" profile={profile({})} />);
    // initial loading spinner visible
    expect(await screen.findByTestId("credits-spinner")).toBeInTheDocument();
    // resolve first fetch
    resolve(sampleCredits as any);
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    expect(screen.queryByTestId("credits-spinner")).not.toBeInTheDocument();
    // click refresh → loading again
    const btn = screen.getByRole("button", { name: /刷新/ });
    let resolve2: (v: any) => void = () => {};
    spy.mockImplementationOnce(() => new Promise((r) => (resolve2 = r)));
    fireEvent.click(btn);
    expect(await screen.findByTestId("credits-spinner")).toBeInTheDocument();
    expect(spy).toHaveBeenCalledTimes(2);
    resolve2(sampleCredits as any);
    await waitFor(() => expect(screen.queryByTestId("credits-spinner")).not.toBeInTheDocument());
  });

  it("shows inline error with retry on failure, not blocking", async () => {
    const spy = vi.spyOn(api, "getCredits").mockRejectedValueOnce(new Error("upstream 401: bad key")).mockResolvedValueOnce(sampleCredits as any);
    render(<SupplierCreditsPanel name="p1" profile={profile({})} />);
    await waitFor(() => expect(screen.getByText(/upstream 401/)).toBeInTheDocument());
    // error should be red text (check class)
    const errEl = screen.getByText(/upstream 401/);
    expect(errEl.className).toMatch(/text-red/);
    // retry button exists
    const retry = screen.getByRole("button", { name: /重试/ });
    expect(retry).toBeInTheDocument();
    // clicking retry re-fetches and shows data
    fireEvent.click(retry);
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    expect(screen.queryByText(/upstream 401/)).not.toBeInTheDocument();
  });

  it("does not render for non opencode-go supplier", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const { container } = render(<SupplierCreditsPanel name="p2" profile={profile({ baseUrl: "https://api.example.com/v1" })} />);
    // should render nothing
    expect(container.innerHTML.trim()).toBe("");
    expect(spy).not.toHaveBeenCalled();
  });

  it("multi-upstream supplier renders single panel and fetches once", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const p = profile({
      baseUrl: "https://api.opencode.ai/v1",
      upstreams: [
        { baseUrl: "https://api.opencode.ai/v1", apiKey: "k1" },
        { baseUrl: "https://other.com/v1", apiKey: "k2" },
      ],
    });
    render(<SupplierCreditsPanel name="multi" profile={p} />);
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith("multi");
    // only one panel, one progress bar
    expect(screen.getAllByTestId("credits-progress-bar")).toHaveLength(1);
    expect(screen.getByText(/主上游/)).toBeInTheDocument();
  });

  it("does not render panel when primary is not opencode.ai even if secondary is", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const p = profile({
      upstreams: [
        { baseUrl: "https://other.com/v1", apiKey: "k1" },
        { baseUrl: "https://api.opencode.ai/v1", apiKey: "k2" },
      ],
    });
    const { container } = render(<SupplierCreditsPanel name="multi2" profile={p} />);
    expect(container.innerHTML.trim()).toBe("");
    expect(spy).not.toHaveBeenCalled();
  });
});

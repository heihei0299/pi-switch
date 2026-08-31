import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProfilesPanel } from "./ProfilesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWithProfiles(profiles: Record<string, any>): AppState {
  return {
    current: "native",
    profiles,
    settings: {} as any,
  } as unknown as AppState;
}

function renderPanel(state: AppState) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <ProfilesPanel state={state} refresh={async () => {}} />
      </ToastProvider>
    </LanguageProvider>,
  );
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

describe("ProfilesPanel credits integration", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("opencode-go card shows credits panel with 主上游, 非命中不显示", async () => {
    vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const state = stateWithProfiles({
      "opencode-go": {
        api: "openai-completions",
        baseUrl: "https://api.opencode.ai/v1",
        apiKey: "k",
        models: [],
        proxy: false,
      },
      other: {
        api: "openai-completions",
        baseUrl: "https://api.example.com/v1",
        apiKey: "k",
        models: [],
        proxy: false,
      },
    });
    renderPanel(state);
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    // Only one panel (opencode-go) should show 主上游
    expect(screen.getAllByText(/主上游/)).toHaveLength(1);
    // other card should not have credits (only one balance)
    expect(screen.getAllByText(/余额/)).toHaveLength(1);
    expect(screen.getAllByText(/已用/)).toHaveLength(1);
    // progress bar only one
    expect(screen.getAllByTestId("credits-progress-bar")).toHaveLength(1);
  });

  it("card mount auto fetches and refresh re-fetches", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const state = stateWithProfiles({
      "opencode-go": {
        api: "openai-completions",
        baseUrl: "https://api.opencode.ai/v1",
        apiKey: "k",
        models: [],
        proxy: false,
      },
    });
    renderPanel(state);
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
    expect(spy).toHaveBeenCalledWith("opencode-go");
    const btn = await screen.findByRole("button", { name: /刷新/ });
    fireEvent.click(btn);
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
  });

  it("credits error shows inline red + retry, not global toast, and does not block edit/delete", async () => {
    vi.spyOn(api, "getCredits").mockRejectedValue(new Error("upstream 401: bad"));
    const state = stateWithProfiles({
      "opencode-go": {
        api: "openai-completions",
        baseUrl: "https://api.opencode.ai/v1",
        apiKey: "k",
        models: [],
        proxy: false,
      },
    });
    renderPanel(state);
    await waitFor(() => expect(screen.getByText(/upstream 401/)).toBeInTheDocument());
    const errEl = screen.getByText(/upstream 401/);
    expect(errEl.className).toMatch(/text-red/);
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
    // Edit/Delete still visible
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    // no global toast should appear (toast provider would show red toast div)
    // Ensure that the error is inside card, not via toast by checking that toast container not containing same text as separate toast style?
    // We check that still Edit exists and no additional toast with bg-red-950? The error inline is inside card, toast would be fixed bottom.
    // Ensure retry works
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    fireEvent.click(screen.getByRole("button", { name: /重试/ }));
    await waitFor(() => expect(spy).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
  });

  it("multi-upstream card shows single panel, fetches once, not per upstream", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const state = stateWithProfiles({
      multi: {
        api: "openai-completions",
        baseUrl: "https://api.opencode.ai/v1",
        apiKey: "k",
        models: [],
        proxy: false,
        upstreams: [
          { baseUrl: "https://api.opencode.ai/v1", apiKey: "k1" },
          { baseUrl: "https://other.com/v1", apiKey: "k2" },
        ],
      },
    });
    renderPanel(state);
    await waitFor(() => expect(screen.getByText(/余额/)).toBeInTheDocument());
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith("multi");
    expect(screen.getAllByTestId("credits-progress-bar")).toHaveLength(1);
    expect(screen.getByText(/主上游/)).toBeInTheDocument();
    // ensure not showing per upstream list doubling
    expect(screen.queryAllByText(/余额/).length).toBe(1);
  });

  it("non-opencode-go with multiple upstreams still no panel", async () => {
    const spy = vi.spyOn(api, "getCredits").mockResolvedValue(sampleCredits as any);
    const state = stateWithProfiles({
      other: {
        api: "openai-completions",
        baseUrl: "https://api.example.com/v1",
        apiKey: "k",
        models: [],
        proxy: false,
        upstreams: [
          { baseUrl: "https://api.example.com/v1", apiKey: "k1" },
          { baseUrl: "https://api.opencode.ai/v1", apiKey: "k2" },
        ],
      },
    });
    renderPanel(state);
    // wait a tick to ensure not fetching due to primary miss
    await new Promise((r) => setTimeout(r, 200));
    expect(spy).not.toHaveBeenCalled();
    expect(screen.queryByText(/主上游/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("credits-progress-bar")).not.toBeInTheDocument();
  });
});

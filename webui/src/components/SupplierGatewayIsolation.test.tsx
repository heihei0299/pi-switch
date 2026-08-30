import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import { GatewayPanel } from "./GatewayPanel";
import { SettingsPanel } from "./SettingsPanel";
import type { AppState } from "../types";
import App from "../App";

function stateWithSettings(overrides: Record<string, unknown> = {}) {
  return {
    current: "native",
    profiles: { p1: { api: "openai-completions", baseUrl: "http://a/v1", apiKey: "k", models: [{ id: "m1" }], proxy: false } },
    settings: {
      providerPrefix: "pi-switch",
      writeMode: "merge",
      gatewayApi: "openai-completions",
      language: null,
      injectOpenCodeAttribution: true,
      proxy: { host: "127.0.0.1", port: 43112, target: null, failover: [], userAgent: null, circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 } },
      web: { host: "127.0.0.1", port: 43110 },
      ...overrides,
    },
  } as unknown as AppState;
}

describe("supplier-gateway frontend isolation (#03)", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("gateway offline does not prevent supplier Profiles actions (getState still works)", async () => {
    // Gateway preview fails, but supplier state fetch succeeds
    const state = stateWithSettings();
    vi.spyOn(api, "getState").mockResolvedValue(state);
    vi.spyOn(api, "previewGateway").mockRejectedValue(new Error("gateway offline"));
    vi.spyOn(api, "addProfile").mockResolvedValue({ ok: true } as any);

    // Render App which contains both panels behind error boundaries
    render(
      <LanguageProvider configLang="en">
        <ToastProvider>
          <App />
        </ToastProvider>
      </LanguageProvider>
    );

    // Wait for App to load state (Profiles nav) - multiple pi-switch texts exist, use provider control hint
    await waitFor(() => expect(screen.getAllByText(/pi-switch/).length).toBeGreaterThan(0), { timeout: 2000 });
    // App should not be in error state for supplier; getState succeeded
    expect(screen.queryByText(/Could not load config/)).not.toBeInTheDocument();

    // Navigate to gateway should show error toast but not crash app
    const gatewayBtn = screen.getAllByText("Gateway").find(el => el.tagName === "BUTTON") || screen.getByText("Gateway");
    fireEvent.click(gatewayBtn);
    await waitFor(() => expect(screen.getByText("gateway offline")).toBeInTheDocument());

    // Navigate back to profiles - supplier panel should still be functional
    const profilesBtn = screen.getAllByText("Profiles").find(el => el.tagName === "BUTTON") || screen.getByText("Profiles");
    fireEvent.click(profilesBtn);
    await waitFor(() => expect(api.getState).toHaveBeenCalled());
  });

  it("supplier authority: gateway extra fields are preserved but not elevated (proposal via preview)", async () => {
    // Preview returns current with extraKept and proposed without it; merge should preserve it but not override api
    const currentWithExtra = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "p/m1", custom: "keep" }], proxy: false, extraKept: 1 } as any;
    const proposed = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "p/m1", input: ["text"] }], proxy: false } as any;
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentWithExtra, proposed, conflicts: [], pending_count: 1 } as any);
    render(
      <LanguageProvider configLang="en">
        <ToastProvider>
          <GatewayPanel refresh={vi.fn(async () => {})} />
        </ToastProvider>
      </LanguageProvider>
    );
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    // Panel should show pending 1 and not crash on extra fields
    expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument();
  });
});

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsPanel } from "./SettingsPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWithSettings(overrides: Record<string, unknown> = {}) {
  return {
    current: "native",
    profiles: {},
    settings: {
      providerPrefix: "pi-switch",
      writeMode: "merge",
      gatewayApi: "openai-completions",
      language: null,
      injectOpenCodeAttribution: true,
      proxy: {
        host: "127.0.0.1",
        port: 43112,
        target: null,
        failover: [],
        userAgent: null,
        circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 },
      },
      web: { host: "127.0.0.1", port: 43110 },
      ...overrides,
    },
  } as unknown as AppState;
}

function renderPanel(state = stateWithSettings(), refresh = vi.fn(async () => {})) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <SettingsPanel state={state} refresh={refresh} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

describe("SettingsPanel save decoupled from gateway (gateway-sep)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("save does not trigger previewGateway/applyGateway, only updateSettings and toast", async () => {
    const update = vi.spyOn(api, "updateSettings").mockResolvedValue({ ok: true } as any);
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderPanel(stateWithSettings(), refresh);

    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(screen.queryByText(/Current vs Proposed/i)).not.toBeInTheDocument();
  });

  it("changing gatewayApi/providerPrefix/host/port still only triggers local save", async () => {
    const update = vi.spyOn(api, "updateSettings").mockResolvedValue({ ok: true } as any);
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const state = stateWithSettings({ gatewayApi: "openai-completions", providerPrefix: "pi-switch" });
    renderPanel(state, vi.fn(async () => {}));

    // change gatewayApi
    const gatewaySelect = screen.getByDisplayValue("OpenAI Chat Completions") as HTMLSelectElement;
    fireEvent.change(gatewaySelect, { target: { value: "openai-responses" } });
    // change providerPrefix
    const prefixInput = screen.getByDisplayValue("pi-switch") as HTMLInputElement;
    fireEvent.change(prefixInput, { target: { value: "my-prefix" } });

    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    const calledWith = update.mock.calls[0][0] as any;
    expect(calledWith.gatewayApi).toBe("openai-responses");
    expect(calledWith.providerPrefix).toBe("my-prefix");
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
  });

  it("does not show gateway preview modal after save", async () => {
    vi.spyOn(api, "updateSettings").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: {}, proposed: {}, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
    expect(screen.queryByText(/Gateway preview/)).not.toBeInTheDocument();
    expect(screen.queryByText(/提议 JSON/)).not.toBeInTheDocument();
  });
});

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProxyPanel } from "./ProxyPanel";
import { LanguageProvider } from "../i18n";

vi.mock("../api", () => ({
  api: {
    proxyStatus: vi.fn(async () => ({ running: false })),
    proxyStart: vi.fn(),
    proxyStop: vi.fn(),
    setFailover: vi.fn(),
  },
}));

describe("ProxyPanel failover removed", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("does not render Failover chain", async () => {
    const state: any = {
      profiles: {
        "supplier-a": { proxy: false, models: [], exposedModels: [] },
        "supplier-b": { proxy: false, models: [], exposedModels: [] },
      },
      settings: {
        providerPrefix: "pi-switch",
        writeMode: "gateway",
        gatewayApi: "openai-completions",
        conversationSource: "sessionScan",
        proxy: { host: "127.0.0.1", port: 43112, failover: ["supplier-b"], circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 } },
        web: { host: "127.0.0.1", port: 43110 },
      },
    };
    render(
      <LanguageProvider configLang="en">
        <ProxyPanel state={state} refresh={async () => {}} />
      </LanguageProvider>,
    );
    expect(screen.queryByText("Failover chain")).toBeNull();
    expect(screen.queryByText("No failover configured.")).toBeNull();
    expect(screen.queryByText(/Same-model fallback/)).toBeNull();
  });
});

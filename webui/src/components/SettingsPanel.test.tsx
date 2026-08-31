import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsPanel } from "./SettingsPanel";
import type { AppState } from "../types";

const updateSettingsMock = vi.fn();

vi.mock("../api", () => ({
  api: {
    updateSettings: (...args: unknown[]) => updateSettingsMock(...args),
  },
}));

vi.mock("./ui", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return {
    ...actual,
    useAction: () => (fn: () => Promise<unknown>) => fn(),
  };
});

vi.mock("../i18n", () => ({
  useI18n: () => ({
    t: (s: string) => s,
    lang: "en",
    setLang: () => {},
  }),
}));

function makeState(overrides: Partial<AppState["settings"]> = {}): AppState {
  return {
    current: null,
    profiles: {},
    settings: {
      providerPrefix: "pi-switch",
      writeMode: "merge",
      language: null,
      proxy: {
        host: "127.0.0.1",
        port: 43112,
        failover: [],
        circuitBreaker: { enabled: true, failureThreshold: 3, cooldownSeconds: 60 },
      },
      web: { host: "127.0.0.1", port: 43110 },
      conversationSource: "sessionScan",
      ...overrides,
    } as AppState["settings"],
  };
}

describe("SettingsPanel conversationSource tri-state", () => {
  beforeEach(() => {
    updateSettingsMock.mockReset();
    updateSettingsMock.mockResolvedValue({});
  });
  afterEach(() => {
    cleanup();
  });

  it("renders tri-state select with 3 mutually exclusive options", async () => {
    render(<SettingsPanel state={makeState({ conversationSource: "sessionScan" })} refresh={async () => {}} />);
    const select = screen.getByLabelText(/Conversation source/i) as HTMLSelectElement;
    expect(select).toBeInTheDocument();
    const options = Array.from(select.querySelectorAll("option")).map((o) => o.value);
    expect(options).toEqual(expect.arrayContaining(["proxy", "sessionScan", "off"]));
    expect(options).toHaveLength(3);
    expect(select.value).toBe("sessionScan");
  });

  it("defaults to sessionScan when missing", async () => {
    const state = makeState();
    // @ts-expect-error missing field simulation
    delete (state.settings as Record<string, unknown>).conversationSource;
    render(<SettingsPanel state={state} refresh={async () => {}} />);
    const select = screen.getByLabelText(/Conversation source/i) as HTMLSelectElement;
    expect(select.value).toBe("sessionScan");
  });

  it("switches mutually exclusively between proxy/sessionScan/off", async () => {
    render(<SettingsPanel state={makeState({ conversationSource: "sessionScan" })} refresh={async () => {}} />);
    const select = screen.getByLabelText(/Conversation source/i) as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "proxy" } });
    expect(select.value).toBe("proxy");
    fireEvent.change(select, { target: { value: "off" } });
    expect(select.value).toBe("off");
    fireEvent.change(select, { target: { value: "sessionScan" } });
    expect(select.value).toBe("sessionScan");
  });

  it("saves selected conversationSource and does not include legacy field", async () => {
    render(<SettingsPanel state={makeState({ conversationSource: "sessionScan" })} refresh={async () => {}} />);
    const select = screen.getByLabelText(/Conversation source/i) as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "off" } });
    const saveBtn = screen.getByRole("button", { name: /Save settings/i });
    fireEvent.click(saveBtn);
    // wait for mock to be called
    await vi.waitFor(() => expect(updateSettingsMock).toHaveBeenCalled());
    const saved = updateSettingsMock.mock.calls[0][0] as Record<string, unknown>;
    expect(saved.conversationSource).toBe("off");
    expect(saved).not.toHaveProperty("injectOpenCodeAttribution");
  });

  it("round-trip preserves conversationSource proxy", async () => {
    render(<SettingsPanel state={makeState({ conversationSource: "proxy" })} refresh={async () => {}} />);
    const select = screen.getByLabelText(/Conversation source/i) as HTMLSelectElement;
    expect(select.value).toBe("proxy");
    const saveBtn = screen.getByRole("button", { name: /Save settings/i });
    fireEvent.click(saveBtn);
    await vi.waitFor(() => expect(updateSettingsMock).toHaveBeenCalled());
    const saved = updateSettingsMock.mock.calls[0][0] as Record<string, unknown>;
    expect(saved.conversationSource).toBe("proxy");
  });
});

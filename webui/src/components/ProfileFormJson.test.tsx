import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProfilesPanel } from "./ProfilesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWithProfile(overrides: Record<string, unknown> = {}) {
  return {
    current: "native",
    profiles: {
      native: {
        api: "openai-completions",
        baseUrl: "https://example.test/v1",
        apiKey: "key123",
        models: [{ id: "m1", input: ["text"], contextWindow: 1000, maxTokens: 100 }],
        proxy: false,
        responsesMode: "auto",
        headers: { "x-custom": "1" },
        compat: { foo: "bar" },
        ...overrides,
      },
    },
    settings: {},
  } as unknown as AppState;
}
function renderPanel(state = stateWithProfile(), refresh = vi.fn(async () => {})) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <ProfilesPanel state={state} refresh={refresh} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

describe("ProfileForm JSON dual editor — T3", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows structured/JSON tabs in ProfileForm", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Name")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "格式化" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "JSON" })).toBeInTheDocument();
  });

  it("JSON tab shows profile JSON with api/baseUrl", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Name")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("profile json");
    expect((ta as HTMLTextAreaElement).value).toContain("openai-completions");
    expect((ta as HTMLTextAreaElement).value).toContain("https://example.test/v1");
    expect((ta as HTMLTextAreaElement).value).toContain("x-custom");
  });

  it("invalid profile JSON disables Save", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Name")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("profile json");
    fireEvent.change(ta, { target: { value: JSON.stringify({ api: "bad", baseUrl: "https://a/v1" }) } });
    await waitFor(() => expect(screen.getByText(/profile\.api is not supported/)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saving from JSON updates profile via api and preserves headers/compat", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    const refresh = vi.fn(async () => {});
    renderPanel(stateWithProfile(), refresh);
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Name")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("profile json");
    const next = JSON.stringify({ api: "openai-responses", baseUrl: "https://new.example.com/v1", apiKey: "newkey", headers: { "x-new": "2" }, compat: { bar: 1 } }, null, 2);
    fireEvent.change(ta, { target: { value: next } });
    await waitFor(() => expect(screen.getByText("✓ JSON valid")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    const calledProfile = (update.mock.calls[0] as any[])[1] as any;
    expect(calledProfile.api).toBe("openai-responses");
    expect(calledProfile.baseUrl).toBe("https://new.example.com/v1");
    expect(calledProfile.headers["x-new"]).toBe("2");
    expect(calledProfile.compat.bar).toBe(1);
    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it("switching JSON -> structured repopulates form fields", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Name")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("profile json");
    const next = JSON.stringify({ api: "anthropic-messages", baseUrl: "https://anthropic.example.com/v1", apiKey: "k" }, null, 2);
    fireEvent.change(ta, { target: { value: next } });
    await waitFor(() => expect(screen.getByText("✓ JSON valid")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "结构化" }));
    await waitFor(() => {
      const selects = screen.getAllByRole("combobox");
      // apiType is second combobox (first is preset, second api, third responsesMode, etc)
      expect(selects[1]).toHaveValue("anthropic-messages");
    });
  });
});

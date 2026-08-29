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
        api: "openai-responses",
        baseUrl: "https://example.test/v1",
        apiKey: "key",
        models: [],
        proxy: false,
        responsesMode: "passthrough",
        ...overrides,
      },
    },
    settings: {},
  } as unknown as AppState;
}

function renderPanel(state = stateWithProfile()) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <ProfilesPanel state={state} refresh={vi.fn(async () => {})} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

describe("provider Responses mode form", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows and preserves the existing Responses mode", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    expect(screen.getAllByRole("combobox")[2]).toHaveValue("passthrough");
    expect(screen.getByText("Responses mode")).toBeInTheDocument();
  });

  it("saves the selected Responses mode through the profile API", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: { api: "openai-responses", baseUrl: "https://example.test/v1", apiKey: "key", models: [], proxy: false }, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(
        "native",
        expect.objectContaining({ responsesMode: "passthrough" }),
        undefined,
      ),
    );
  });

  it("blocks a passthrough mode on a Chat Completions provider", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));

    fireEvent.change(screen.getAllByRole("combobox")[1], {
      target: { value: "openai-completions" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(screen.getByText("passthrough requires openai-responses")).toBeInTheDocument(),
    );
    expect(update).not.toHaveBeenCalled();
  });

  it("shows the effective Responses mode on the provider list", () => {
    renderPanel(stateWithProfile({ responsesMode: "auto" }));
    expect(screen.getByText("Responses: passthrough")).toBeInTheDocument();
  });

  it("maps auto to conversion for a Chat Completions provider", () => {
    renderPanel(stateWithProfile({ api: "openai-completions", responsesMode: "auto" }));
    expect(screen.getByText("Responses: convert")).toBeInTheDocument();
  });
});

describe("API type human-readable labels", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders 4 human-readable labels with IDs as values", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    const apiSelect = screen.getAllByRole("combobox")[1] as HTMLSelectElement;
    const options = Array.from(apiSelect.querySelectorAll("option"));
    expect(options).toHaveLength(4);
    expect(options.map((o) => o.value)).toEqual([
      "openai-completions",
      "openai-responses",
      "anthropic-messages",
      "google-generative-ai",
    ]);
    expect(options.map((o) => o.textContent?.trim())).toEqual([
      "OpenAI Chat Completions",
      "OpenAI Responses",
      "Anthropic Messages",
      "Google Gemini",
    ]);
    expect(apiSelect.value).toBe("openai-responses");
  });

  it("echoes the existing provider api correctly", async () => {
    renderPanel(stateWithProfile({ api: "google-generative-ai" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    expect(screen.getAllByRole("combobox")[1]).toHaveValue("google-generative-ai");
    // label should be selected
    const apiSelect = screen.getAllByRole("combobox")[1] as HTMLSelectElement;
    const selected = apiSelect.options[apiSelect.selectedIndex];
    expect(selected.textContent?.trim()).toBe("Google Gemini");
  });

  it("builds profile with the selected api id on save", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: { api: "anthropic-messages", baseUrl: "https://example.test/v1", apiKey: "key", models: [], proxy: false }, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderPanel(stateWithProfile({ api: "openai-completions", responsesMode: "auto" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    fireEvent.change(screen.getAllByRole("combobox")[1], { target: { value: "anthropic-messages" } });
    await waitFor(() => expect(screen.getAllByRole("combobox")[1]).toHaveValue("anthropic-messages"));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    const calledWith = (update.mock.calls[0] as any[])[1];
    expect(calledWith.api).toBe("anthropic-messages");
  });

  it("preserves unknown api value without silent fallback", async () => {
    renderPanel(stateWithProfile({ api: "unknown-api" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    const apiSelect = screen.getAllByRole("combobox")[1] as HTMLSelectElement;
    expect(apiSelect.value).toBe("unknown-api");
    const options = Array.from(apiSelect.querySelectorAll("option"));
    expect(options.some((o) => o.value === "unknown-api")).toBe(true);
  });

  it("shows help text for the interface format selector", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Select the API interface format for the AI service.")).toBeInTheDocument());
  });
});

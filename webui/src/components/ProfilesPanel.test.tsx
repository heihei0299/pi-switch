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

function renderPanel(state = stateWithProfile(), refresh = vi.fn(async () => {})) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <ProfilesPanel state={state} refresh={refresh} />
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

  it("saves the selected Responses mode through the profile API without gateway preview", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderPanel(stateWithProfile(), refresh);
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(
      "native",
      expect.objectContaining({ responsesMode: "passthrough" }),
      undefined,
    ));
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    // should toast local save
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(screen.queryByText(/Current vs Proposed/i)).not.toBeInTheDocument();
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

  it("builds profile with the selected api id on save without gateway preview", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({});
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderPanel(stateWithProfile({ api: "openai-completions", responsesMode: "auto" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getAllByRole("combobox")).toHaveLength(4));
    fireEvent.change(screen.getAllByRole("combobox")[1], { target: { value: "anthropic-messages" } });
    await waitFor(() => expect(screen.getAllByRole("combobox")[1]).toHaveValue("anthropic-messages"));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    const calledWith = (update.mock.calls[0] as any[])[1];
    expect(calledWith.api).toBe("anthropic-messages");
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
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

describe("Profiles save decoupled from gateway (gateway-sep)", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("ModelsModal save does not trigger previewGateway/applyGateway, only updateModels+expose and toast", async () => {
    const updateModels = vi.spyOn(api, "updateModels").mockResolvedValue({ ok: true } as any);
    const expose = vi.spyOn(api, "expose").mockResolvedValue({ ok: true } as any);
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "fetchModels").mockResolvedValue({ models: [], enrich: undefined } as any);
    const refresh = vi.fn(async () => {});
    const state = stateWithProfile({ models: [{ id: "m1", input: ["text"], contextWindow: 1000, maxTokens: 100 }], exposedModels: [] });
    renderPanel(state, refresh);
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateModels).toHaveBeenCalled());
    await waitFor(() => expect(expose).toHaveBeenCalled());
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
    expect(screen.queryByText(/Current vs Proposed/i)).not.toBeInTheDocument();
    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it("Add profile save does not trigger preview/apply", async () => {
    const add = vi.spyOn(api, "addProfile").mockResolvedValue({});
    const preview = vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: {}, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderPanel(stateWithProfile(), refresh);
    // click Add profile
    fireEvent.click(screen.getByText("+ Add profile"));
    await waitFor(() => expect(screen.getByPlaceholderText("my-provider")).toBeInTheDocument());
    const nameInput = screen.getByPlaceholderText("my-provider") as HTMLInputElement;
    fireEvent.change(nameInput, { target: { value: "new-provider" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(add).toHaveBeenCalledWith("new-provider", expect.any(Object)));
    expect(preview).not.toHaveBeenCalled();
    expect(apply).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText("已保存到本地，需到网关发布")).toBeInTheDocument());
  });
});

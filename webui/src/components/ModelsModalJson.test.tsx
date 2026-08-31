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
        apiKey: "key",
        models: [
          { id: "m1", name: "M1", input: ["text"], contextWindow: 1000, maxTokens: 100, customField: "keep", cost: { input: 1, output: 2, cacheRead: 0, cacheWrite: 0 } },
        ],
        proxy: false,
        exposedModels: ["m1"],
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

describe("ModelsModal JSON dual editor — T2", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
    vi.spyOn(api, "fetchModels").mockResolvedValue({ models: [], enrich: undefined } as any);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows structured/JSON tabs in ModelsModal", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "格式化" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "JSON" })).toBeInTheDocument();
  });

  it("JSON tab shows models array JSON with passthrough", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("models json");
    expect(ta).toBeInTheDocument();
    expect((ta as HTMLTextAreaElement).value).toContain('"id": "m1"');
    expect((ta as HTMLTextAreaElement).value).toContain("customField");
  });

  it("invalid JSON disables Save and shows error", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("models json");
    fireEvent.change(ta, { target: { value: '[{"id": ""}]' } });
    await waitFor(() => expect(screen.getByText(/model\.id must not be empty/)).toBeInTheDocument());
    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();
  });

  it("saving from JSON preserves passthrough and calls updateModels with normalized camelCase", async () => {
    const updateModels = vi.spyOn(api, "updateModels").mockResolvedValue({ ok: true } as any);
    const expose = vi.spyOn(api, "expose").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderPanel(stateWithProfile(), refresh);
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("models json");
    const next = JSON.stringify([{ id: "m2", name: "M2", context_window: 200, cost: { input: 1, output: 2, cacheRead: 0, cacheWrite: 0 }, extraKeep: "yes" }], null, 2);
    fireEvent.change(ta, { target: { value: next } });
    await waitFor(() => expect(screen.getByText("✓ JSON valid")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateModels).toHaveBeenCalled());
    const calledModels = (updateModels.mock.calls[0] as any[])[1] as any[];
    expect(calledModels[0].id).toBe("m2");
    expect(calledModels[0].contextWindow).toBe(200);
    expect((calledModels[0] as any).context_window).toBeUndefined();
    expect((calledModels[0] as any).extraKeep).toBe("yes");
    await waitFor(() => expect(refresh).toHaveBeenCalled());
  });

  it("switching JSON -> structured rebuilds drafts and keeps passthrough preview", async () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Models" }));
    await waitFor(() => expect(screen.getByText(/Model config/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const ta = await screen.findByLabelText("models json");
    const next = JSON.stringify([{ id: "m3", name: "M3", customField: "keep" }], null, 2);
    fireEvent.change(ta, { target: { value: next } });
    await waitFor(() => expect(screen.getByText("✓ JSON valid")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "结构化" }));
    await waitFor(() => expect(screen.getByDisplayValue("m3")).toBeInTheDocument());
    expect(screen.getByDisplayValue("M3")).toBeInTheDocument();
  });
});

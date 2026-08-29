import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProfilesPanel } from "./ProfilesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWith(overrides: Record<string, unknown> = {}) {
  return {
    current: "native",
    profiles: {
      native: {
        api: "openai-responses",
        baseUrl: "https://example.test/v1",
        apiKey: "key",
        models: [],
        proxy: false,
        ...overrides,
      },
    },
    settings: {},
  } as unknown as AppState;
}
function renderPanel(state = stateWith(), refresh = vi.fn(async () => {})) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <ProfilesPanel state={state} refresh={refresh} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

describe("ProfilesPanel Upstream list增删 (has_upstreams/resolved_upstreams)", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPresets").mockResolvedValue([]);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows single fallback when no upstreams, and can convert to multi upstream", async () => {
    renderPanel(stateWith());
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText(/single fallback/i)).toBeInTheDocument());
    expect(screen.getByText("Manage upstreams")).toBeInTheDocument();
    // convert
    fireEvent.click(screen.getByText("Manage upstreams"));
    await waitFor(() => expect(screen.getByText(/Upstream #1/)).toBeInTheDocument());
    expect(screen.getByPlaceholderText("upstream-a")).toBeInTheDocument();
  });

  it("renders existing upstreams and allows add/remove", async () => {
    renderPanel(stateWith({ upstreams: [{ baseUrl: "http://a/v1", apiKey: "k1", weight: 2, name: "a" } as any] }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText(/Upstream #1/)).toBeInTheDocument());
    expect(screen.getByDisplayValue("http://a/v1")).toBeInTheDocument();
    expect(screen.getByDisplayValue("k1")).toBeInTheDocument();
    expect(screen.getByDisplayValue("2")).toBeInTheDocument();
    expect(screen.getByDisplayValue("a")).toBeInTheDocument();
    // add second upstream
    fireEvent.click(screen.getByText("+ Add upstream"));
    await waitFor(() => expect(screen.getByText("Upstream #2")).toBeInTheDocument());
    // remove first
    const removeBtns = screen.getAllByRole("button", { name: "Remove" });
    fireEvent.click(removeBtns[0]);
    await waitFor(() => expect(screen.queryByDisplayValue("http://a/v1")).not.toBeInTheDocument());
    expect(screen.getByText("Upstream #1")).toBeInTheDocument();
  });

  it("saving with upstreams sends upstreams payload and keeps single field compat", async () => {
    const update = vi.spyOn(api, "updateProfile").mockResolvedValue({} as any);
    renderPanel(stateWith({ baseUrl: "http://a/v1", apiKey: "k1" }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Manage upstreams")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Manage upstreams"));
    await waitFor(() => expect(screen.getByText("Upstream #1")).toBeInTheDocument());
    // edit first upstream baseUrl
    const input = screen.getByDisplayValue("http://a/v1") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "http://b/v1" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    const payload = (update.mock.calls[0] as any[])[1];
    expect(payload.upstreams).toBeDefined();
    expect(payload.upstreams[0].baseUrl).toBe("http://b/v1");
    expect(payload.baseUrl).toBe("http://b/v1");
  });

  it("Use single button falls back to single fields", async () => {
    renderPanel(stateWith({ upstreams: [{ baseUrl: "http://a/v1", apiKey: "k1" } as any] }));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Upstream #1")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Use single"));
    await waitFor(() => expect(screen.getByPlaceholderText("https://api.example.com/v1")).toBeInTheDocument());
    expect(screen.queryByText("Upstream #1")).not.toBeInTheDocument();
  });

  it("display shows upstream count via resolvedUpstreams", async () => {
    renderPanel(stateWith({ baseUrl: "http://a/v1", upstreams: [{ baseUrl: "http://a/v1", apiKey: "k1" } as any, { baseUrl: "http://b/v1", apiKey: "k2" } as any] }));
    expect(screen.getByText(/2 upstream\(s\)/)).toBeInTheDocument();
  });
});

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProfilesPanel } from "./ProfilesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWithProfile(api: string) {
  return {
    current: "p",
    profiles: {
      p: { api, baseUrl: "https://example.test/v1", apiKey: "k", responsesMode: "auto", upstreams: [] },
    },
    settings: {},
  } as unknown as AppState;
}

// The Responses mode select must offer only the modes the backend capability
// reports for the selected api, instead of a hardcoded three-item list.
describe("ProfilesPanel responsesMode options follow the capability", () => {
  beforeEach(() => vi.spyOn(api, "getPresets").mockResolvedValue([]));
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("offers only auto for anthropic-messages", async () => {
    render(
      <LanguageProvider configLang="en">
        <ToastProvider>
          <ProfilesPanel state={stateWithProfile("anthropic-messages")} refresh={vi.fn(async () => {})} />
        </ToastProvider>
      </LanguageProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Responses mode")).toBeInTheDocument());
    const select = screen.getByDisplayValue(/auto —/);
    const values = Array.from(select.querySelectorAll("option")).map((o) => (o as HTMLOptionElement).value);
    expect(values).toEqual(["auto"]);
  });
});

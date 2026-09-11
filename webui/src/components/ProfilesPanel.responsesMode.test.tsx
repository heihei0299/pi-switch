import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProfilesPanel } from "./ProfilesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";
import type { AppState } from "../types";

function stateWithProfile(api: string, responsesMode = "auto") {
  return {
    current: "p",
    profiles: {
      p: { api, baseUrl: "https://example.test/v1", apiKey: "k", responsesMode, upstreams: [] },
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

  // 与 api 选择器同一原则：旧配置里不属于该 api 的 mode 仍要显示并回显（否则 profile
  // 打不开也读不出自己的值），但不能被重新选中——它与当前 api 的组合是写入口会拒绝的
  // 那种组合。合法 mode 保持可选，所以修好 profile 的路径没有断。
  it("shows a legacy incompatible mode but keeps it unselectable", async () => {
    render(
      <LanguageProvider configLang="en">
        <ToastProvider>
          <ProfilesPanel
            state={stateWithProfile("openai-completions", "passthrough")}
            refresh={vi.fn(async () => {})}
          />
        </ToastProvider>
      </LanguageProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(screen.getByText("Responses mode")).toBeInTheDocument());

    const select = screen.getByDisplayValue(/passthrough —/) as HTMLSelectElement;
    expect(select.value).toBe("passthrough");
    const options = Array.from(select.querySelectorAll("option")) as HTMLOptionElement[];
    expect(options.find((o) => o.value === "passthrough")?.disabled).toBe(true);
    for (const mode of ["auto", "convert"]) {
      expect(options.find((o) => o.value === mode)?.disabled).toBe(false);
    }
  });
});

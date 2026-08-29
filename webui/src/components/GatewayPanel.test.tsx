import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GatewayPanel } from "./GatewayPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";

function renderGateway(refresh = vi.fn(async () => {})) {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <GatewayPanel refresh={refresh} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

const currentGw = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "p/m1" }], proxy: false };
const proposedGw = { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "p/m1" }, { id: "p/m2" }], proxy: false };

describe("GatewayPanel gateway-sep", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  it("shows Current vs Proposed status bar with diff and pending count", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: proposedGw, conflicts: [] } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    // status bar shows changed for models
    expect(screen.getByText(/\+0 added/)).toBeInTheDocument();
    expect(screen.getByText(/-0 removed/)).toBeInTheDocument();
    expect(screen.getByText(/~1 changed/)).toBeInTheDocument();
    expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument();
    expect(screen.getByText(/上次发布时间/)).toBeInTheDocument();
  });

  it("shows mismatch banner on first entry when diff non-empty, not auto apply", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: proposedGw, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/检测到本地与 Pi 网关不一致/)).toBeInTheDocument());
    expect(screen.getByText("立即同步")).toBeInTheDocument();
    // default not auto write
    expect(apply).not.toHaveBeenCalled();
  });

  it("does not show mismatch banner when no diff", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: currentGw, conflicts: [] } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect(screen.queryByText(/检测到本地与 Pi 网关不一致/)).not.toBeInTheDocument();
  });

  it("clicking 应用到 Pi calls PUT /models/gateway and on success refresh and clears pending", async () => {
    vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce({ current: currentGw, proposed: proposedGw, conflicts: [] } as any)
      .mockResolvedValueOnce({ current: proposedGw, proposed: proposedGw, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderGateway(refresh);
    await waitFor(() => expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    // apply payload should be parseable gateway
    const payload = apply.mock.calls[0][0] as any;
    expect(payload.api).toBe("openai-completions");
    expect(payload.baseUrl).toBe("http://127.0.0.1:43112/v1");
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/待发布数: 0/)).toBeInTheDocument());
    // lastPublishAt should be set (not 尚未发布)
    await waitFor(() => expect(screen.queryByText(/上次发布时间: 尚未发布/)).not.toBeInTheDocument());
    expect(screen.queryByText(/检测到本地与 Pi 网关不一致/)).not.toBeInTheDocument();
  });

  it("failed apply retains config and does not refresh or clear pending", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: proposedGw, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockRejectedValue(new Error("apply failed"));
    const refresh = vi.fn(async () => {});
    renderGateway(refresh);
    await waitFor(() => expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument());
    // remember liveJson before
    const pre = (screen.getByText(/"api":/) as HTMLElement).textContent;
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(screen.getByText("apply failed")).toBeInTheDocument());
    expect(refresh).not.toHaveBeenCalled();
    // pending still 1
    expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument();
    // config retained (liveJson still same)
    expect((screen.getByText(/"api":/) as HTMLElement).textContent).toBe(pre);
  });

  it("shows 上次发布时间 after successful publish and persists", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: null, proposed: proposedGw, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    // Initially no current -> shows 尚未发布
    renderGateway();
    await waitFor(() => expect(screen.getByText(/上次发布时间: 尚未发布/)).toBeInTheDocument());
    cleanup();
    // after publish, should show timestamp
    vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce({ current: null, proposed: proposedGw, conflicts: [] } as any)
      .mockResolvedValueOnce({ current: proposedGw, proposed: proposedGw, conflicts: [] } as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh2 = vi.fn(async () => {});
    renderGateway(refresh2);
    await waitFor(() => expect(screen.getByRole("button", { name: "应用到 Pi" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(refresh2).toHaveBeenCalled());
    // after reload, last publish should be shown not "尚未发布"
    await waitFor(() => expect(screen.queryByText(/上次发布时间: 尚未发布/)).not.toBeInTheDocument());
  });

  it("dismissing mismatch banner does not auto apply", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: proposedGw, conflicts: [] } as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/检测到本地与 Pi 网关不一致/)).toBeInTheDocument());
    fireEvent.click(screen.getByText("稍后"));
    await waitFor(() => expect(screen.queryByText(/检测到本地与 Pi 网关不一致/)).not.toBeInTheDocument());
    expect(apply).not.toHaveBeenCalled();
  });
});

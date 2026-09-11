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

const channelGateway = (models: Array<Record<string, unknown>>) => ({
  "oc/chat": {
    api: "openai-completions",
    baseUrl: "http://127.0.0.1:43112/v1",
    models,
    proxy: false,
  },
});
const currentGw = channelGateway([{ id: "m1" }]);
const proposedGw = channelGateway([{ id: "m1" }, { id: "m2" }]);

function backendPreview(
  current: Record<string, unknown> | null,
  proposed: Record<string, unknown>,
  extra: Record<string, unknown> = {},
) {
  return {
    current,
    proposed,
    conflicts: [],
    pending_count: current === proposed ? 0 : 1,
    diff: { added: [], removed: [], changed: [] },
    groups: [],
    removed: [],
    ...extra,
  };
}

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
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(currentGw, proposedGw, {
      diff: { added: ["oc/chat/m2"], removed: [], changed: [] },
    }) as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    // status bar shows a newly added bare model under the channel provider
    expect(screen.getByText(/\+1 added/)).toBeInTheDocument();
    expect(screen.getByText(/-0 removed/)).toBeInTheDocument();
    expect(screen.getByText(/~0 changed/)).toBeInTheDocument();
    expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument();
    expect(screen.getByText(/上次发布时间/)).toBeInTheDocument();
  });


  it("does not show mismatch banner when no diff", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(currentGw, currentGw) as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect(screen.queryByText(/检测到本地与 Pi 网关不一致/)).not.toBeInTheDocument();
  });

  it("clicking 应用到 Pi calls PUT /models/gateway and on success refresh and clears pending", async () => {
    vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce(backendPreview(currentGw, proposedGw, {
        diff: { added: ["oc/chat/m2"], removed: [], changed: [] },
      }) as any)
      .mockResolvedValueOnce(backendPreview(proposedGw, proposedGw) as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh = vi.fn(async () => {});
    renderGateway(refresh);
    await waitFor(() => expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument());
    // raw JSON 同步后按钮才可点，避免负载高时点到校验失败的旧状态
    const applyBtn = screen.getByRole("button", { name: "应用到 Pi" });
    await waitFor(() => expect(applyBtn).toBeEnabled());
    fireEvent.click(applyBtn);
    await waitFor(() => expect(apply).toHaveBeenCalled());
    // apply payload should be parseable gateway
    const payload = apply.mock.calls[0][0] as any;
    expect(payload.providers["oc/chat"].api).toBe("openai-completions");
    expect(payload.providers["oc/chat"].baseUrl).toBe("http://127.0.0.1:43112/v1");
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/待发布数: 0/)).toBeInTheDocument());
    // lastPublishAt should be set (not 尚未发布)
    await waitFor(() => expect(screen.queryByText(/上次发布时间: 尚未发布/)).not.toBeInTheDocument());
    expect(screen.queryByText(/检测到本地与 Pi 网关不一致/)).not.toBeInTheDocument();
  });

  it("does not mutate proxy settings from the gateway base URL", async () => {
    const gateway = {
      "oc/chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:8317/v1",
        models: [{ id: "new-model" }],
        proxy: false,
      },
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(gateway, gateway) as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const getState = vi.spyOn(api, "getState").mockResolvedValue({
      settings: { proxy: { host: "127.0.0.1", port: 43112 } },
    } as any);
    const updateSettings = vi.spyOn(api, "updateSettings").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByRole("button", { name: "应用到 Pi" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    expect(getState).not.toHaveBeenCalled();
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it("preserves provider compat when applying a gateway", async () => {
    const gateway = {
      "oc/chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        compat: { sendSessionAffinityHeaders: true },
        models: [{ id: "new-model" }],
        proxy: false,
      },
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(gateway, gateway) as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByRole("button", { name: "应用到 Pi" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    expect(payload.providers["oc/chat"].compat).toEqual({ sendSessionAffinityHeaders: true });
  });

  it("failed apply retains config and does not refresh or clear pending", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(currentGw, proposedGw, {
      diff: { added: ["oc/chat/m2"], removed: [], changed: [] },
    }) as any);
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
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(null, proposedGw, {
      diff: { added: ["oc/chat/m1", "oc/chat/m2"], removed: [], changed: [] },
    }) as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    // Initially no current -> shows 尚未发布
    renderGateway();
    await waitFor(() => expect(screen.getByText(/上次发布时间: 尚未发布/)).toBeInTheDocument());
    cleanup();
    // after publish, should show timestamp
    vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce(backendPreview(null, proposedGw, {
        diff: { added: ["oc/chat/m1", "oc/chat/m2"], removed: [], changed: [] },
      }) as any)
      .mockResolvedValueOnce(backendPreview(proposedGw, proposedGw) as any);
    vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    const refresh2 = vi.fn(async () => {});
    renderGateway(refresh2);
    await waitFor(() => expect(screen.getByRole("button", { name: "应用到 Pi" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(refresh2).toHaveBeenCalled());
    // after reload, last publish should be shown not "尚未发布"
    await waitFor(() => expect(screen.queryByText(/上次发布时间: 尚未发布/)).not.toBeInTheDocument());
  });


  it("shows conflicts when preview returns conflicts", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({
      ...backendPreview(currentGw, proposedGw),
      conflicts: ["baseUrl", "models"],
    } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const conflictEl = screen.getByText(/冲突:/);
    expect(conflictEl).toBeInTheDocument();
    expect(conflictEl.textContent).toContain("baseUrl");
    expect(conflictEl.textContent).toContain("models");
  });

  it("gateway offline error is caught and does not crash panel (shows toast, retains loading fallback)", async () => {
    vi.spyOn(api, "previewGateway").mockRejectedValue(new Error("gateway offline"));
    renderGateway();
    await waitFor(() => expect(screen.getByText("gateway offline")).toBeInTheDocument());
    // panel should not crash: status bar still renders (empty) or at least not throw
    expect(screen.getByText(/Current vs Proposed/) || screen.getByText("gateway offline")).toBeTruthy();
  });
});

describe("GatewayPanel supplier/channel groups + secondary selection", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });
  const groupedPreview = {
    current: {
      "sup/main": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [{ id: "m1" }],
        proxy: false,
      },
    },
    proposed: {
      "sup/main": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [{ id: "m1" }],
        proxy: false,
      },
      "sup/bk": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43113/v1",
        models: [{ id: "b1" }],
        proxy: false,
      },
    },
    conflicts: [],
    pending_count: 1,
    diff: { added: ["sup/bk/b1"], removed: [], changed: [] },
    groups: [
      { supplier: "sup", channel: "main", models: [{ id: "m1", status: "published" }] },
      { supplier: "sup", channel: "bk", models: [{ id: "b1", status: "pending" }] },
    ],
    removed: ["ghost/x"],
  };

  function renderGrouped() {
    vi.spyOn(api, "previewGateway").mockResolvedValue(groupedPreview as any);
    renderGateway();
  }

  it("groups candidates by supplier/channel with publish status", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / main")).toBeInTheDocument());
    expect(screen.getByText("sup / bk")).toBeInTheDocument();
    expect(screen.getByText("已发布")).toBeInTheDocument();
    expect(screen.getByText("待发布")).toBeInTheDocument();
    expect(screen.getByText("ghost/x")).toBeInTheDocument();
  });

  it("pending candidates default unchecked; checking includes them in the apply payload", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "getState").mockResolvedValue({ settings: { gatewayApi: "openai-completions", proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    // 已发布默认勾选，没发布的默认不勾选
    expect(screen.getByRole("checkbox", { name: /sup\/main\/m1/ })).toBeChecked();
    const box = screen.getByRole("checkbox", { name: /sup\/bk\/b1/ });
    expect(box).not.toBeChecked();
    fireEvent.click(box);
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const providers = payload.providers as Record<string, { models: Array<{ id: string }> }>;
    const allIds: string[] = [];
    for (const [key, prov] of Object.entries(providers ?? {})) {
      for (const m of prov.models) allIds.push(`${key}/${m.id}`);
    }
    expect(allIds).toContain("sup/main/m1");
    expect(allIds).toContain("sup/bk/b1");
    expect(allIds).not.toContain("ghost/x");
  });

  it("rejects publishing an empty selection when no gateway model is published", async () => {
    const preview = {
      ...groupedPreview,
      current: {},
      pending_count: 2,
      diff: { added: ["sup/main/m1", "sup/bk/b1"], removed: [], changed: [] },
      groups: groupedPreview.groups.map((group) => ({
        ...group,
        models: group.models.map((model) => ({ ...model, status: "pending" as const })),
      })),
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(preview as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();

    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));

    await waitFor(() => expect(screen.getByText("请先勾选至少一个模型")).toBeInTheDocument());
    expect(apply).not.toHaveBeenCalled();
  });

  it("keeps the full proposal while adding multiple pending models", async () => {
    const models = [{ id: "m1" }, { id: "m2" }];
    const groups = [{
      supplier: "sup",
      channel: "main",
      gatewayProvider: "pi-switch-chat",
      models: models.map((model) => ({ ...model, status: "pending" as const })),
    }];
    const preview = vi.spyOn(api, "previewGateway").mockImplementation(async (input) => {
      const selected = input?.selected ?? [];
      const selectedIds = new Set(selected.map((item) => item.model));
      const selectedModels = selected.length === 0
        ? models
        : models.filter((model) => selectedIds.has(model.id));
      return {
        current: {},
        proposed: {
          "pi-switch-chat": {
            api: "openai-completions",
            baseUrl: "http://127.0.0.1:43112/v1",
            models: selectedModels,
            proxy: false,
          },
        },
        conflicts: [],
        pending_count: selectedModels.length,
        diff: { added: [], removed: [], changed: [] },
        groups,
        removed: [],
      } as any;
    });
    renderGateway();

    await waitFor(() => expect(screen.getByRole("checkbox", { name: "sup/main/m1" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("checkbox", { name: "sup/main/m1" }));
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "sup/main/m1" })).toBeChecked());
    fireEvent.click(screen.getByRole("checkbox", { name: "sup/main/m2" }));
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "sup/main/m2" })).toBeChecked());

    const secondToggle = preview.mock.calls[2][0] as any;
    expect(secondToggle.draft.providers["pi-switch-chat"].models).toEqual(models);
  });

  it("subset pending follows the selection", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
		// Backend pending_count is the canonical count for the initial full proposal.
		await waitFor(() => expect(screen.getByText(/勾选子集待发布：1/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("checkbox", { name: /sup\/bk\/b1/ }));
    // 勾选 b1：子集多出待发布 → 1
    await waitFor(() => expect(screen.getByText(/勾选子集待发布：1/)).toBeInTheDocument());
  });
});

describe("GatewayPanel canonical draft", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  it("shows the persisted gateway in the editor on initial load", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(currentGw, proposedGw) as any);
    renderGateway();

    await waitFor(() => {
      expect(screen.getByLabelText("gateway json")).toHaveValue(
        JSON.stringify({ providers: currentGw }, null, 2),
      );
    });
    expect((screen.getByLabelText("gateway json") as HTMLTextAreaElement).value).not.toContain("m2");
  });

  it("renders structured preview and publishes the backend proposal", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(backendPreview(currentGw, proposedGw, {
      diff: { added: ["oc/chat/m2"], removed: [], changed: [] },
    }) as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect(screen.getByDisplayValue("m1")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "remove" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    expect(apply.mock.calls[0][0]).toEqual({ providers: proposedGw });
  });

  it("edits gateway and model metadata in the structured view", async () => {
    const gateway = {
      "pi-switch-chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [{
          id: "m1",
          name: "Provider name",
          contextWindow: 100,
          maxTokens: 10,
          input: ["text"],
          reasoning: false,
          cost: { input: 0.1, output: 0.2, cacheRead: 0.03, cacheWrite: 0 },
          compat: { supportsDeveloperRole: false },
          extra: { source: "catalog" },
        }],
        proxy: false,
      },
    };
    const preview = vi.spyOn(api, "previewGateway").mockImplementation(async (input) => {
      const draft = input?.draft && typeof input.draft === "object" && !Array.isArray(input.draft)
        ? (input.draft as Record<string, unknown>).providers as Record<string, unknown>
        : gateway;
      return backendPreview(gateway, draft, { groups: [] }) as any;
    });
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);

    renderGateway();
    await waitFor(() => expect(screen.getByDisplayValue("Provider name")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Toggle model details" }));
    expect(screen.getByText(/Context window/)).toBeInTheDocument();
    expect(screen.getByText(/Max tokens/)).toBeInTheDocument();
    const metadata = screen.getByLabelText("Gateway metadata");
    fireEvent.change(screen.getByDisplayValue("Provider name"), { target: { value: "Gateway name" } });
    fireEvent.change(screen.getByDisplayValue("100"), { target: { value: "200" } });
    fireEvent.change(screen.getByDisplayValue("10"), { target: { value: "20" } });
    screen.getAllByRole("switch").forEach((control) => fireEvent.click(control));
    fireEvent.click(screen.getByRole("button", { name: /Cost/ }));
    fireEvent.change(screen.getByDisplayValue("0.1"), { target: { value: "0.5" } });
    fireEvent.change(metadata, {
      target: { value: JSON.stringify({ compat: { custom: true }, extra: { source: "gateway" } }, null, 2) },
    });

    await waitFor(() => expect((screen.getByLabelText("gateway json") as HTMLTextAreaElement).value).toContain("Gateway name"));
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());

    const model = (apply.mock.calls[0][0] as any).providers["pi-switch-chat"].models[0];
    expect(model).toMatchObject({
      id: "m1",
      name: "Gateway name",
      contextWindow: 200,
      maxTokens: 20,
      reasoning: true,
      input: ["text", "image"],
      cost: { input: 0.5, output: 0.2, cacheRead: 0.03, cacheWrite: 0 },
      compat: { custom: true },
      extra: { source: "gateway" },
    });
    expect(preview).toHaveBeenCalled();
  });

  it("sends edited JSON to backend preview before publishing", async () => {
    const preview = vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce(backendPreview(currentGw, proposedGw, {
        diff: { added: ["oc/chat/m2"], removed: [], changed: [] },
      }) as any)
      .mockResolvedValue(backendPreview(proposedGw, proposedGw) as any);
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const edited = {
      providers: {
        "pi-switch-chat": {
          api: "openai-completions",
          baseUrl: "http://127.0.0.1:43112/v1",
          apiKey: "pi-switch-proxy",
          models: [{ id: "edited-model" }],
          proxy: false,
        },
      },
    };
    fireEvent.change(screen.getByLabelText("gateway json"), {
      target: { value: JSON.stringify(edited) },
    });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    expect(preview.mock.calls).toContainEqual([{ draft: edited }]);
    expect(apply.mock.calls[0][0]).toEqual({ providers: proposedGw });
  });

  it("shows persisted legacy ids on initial load", async () => {
    const legacyCurrent = {
      "pi-switch": {
        api: "openai-responses",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [{ id: "oc/responses/gpt-5.6-luna" }],
        proxy: false,
      },
    };
    const canonicalProposed = {
      "oc/responses": {
        api: "openai-responses",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [{ id: "gpt-5.6-luna" }],
        proxy: false,
      },
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue({
      current: legacyCurrent,
      proposed: canonicalProposed,
      conflicts: [],
      pending_count: 1,
      diff: { added: [], removed: ["pi-switch/oc/responses/gpt-5.6-luna"], changed: [] },
      groups: [],
      removed: ["pi-switch/oc/responses/gpt-5.6-luna"],
    } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect(screen.getByDisplayValue("oc/responses/gpt-5.6-luna")).toBeInTheDocument();
    expect(screen.queryByDisplayValue("gpt-5.6-luna")).not.toBeInTheDocument();
  });
});


describe("GatewayPanel unchecked persistence", () => {
  beforeEach(() => {
    if (!window.localStorage) {
      const store = new Map<string, string>();
      Object.defineProperty(window, "localStorage", {
        value: {
          getItem: (k: string) => (store.has(k) ? (store.get(k) as string) : null),
          setItem: (k: string, v: string) => { store.set(k, String(v)); },
          removeItem: (k: string) => { store.delete(k); },
          clear: () => store.clear(),
        },
        configurable: true,
      });
    }
    window.localStorage.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  const persistPreview = {
    current: { "sup/main": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "m1" }], proxy: false } },
    proposed: {
      "sup/main": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "m1" }], proxy: false },
      "sup/bk": { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "b1" }], proxy: false },
    },
    conflicts: [],
    pending_count: 1,
    diff: { added: [], removed: [], changed: [] },
    groups: [
      { supplier: "sup", channel: "main", models: [{ id: "m1", status: "published" }] },
      { supplier: "sup", channel: "bk", models: [{ id: "b1", status: "pending" }] },
    ],
    removed: [],
  };

  it("unchecking a candidate survives reload", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(persistPreview as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    const box = screen.getByRole("checkbox", { name: /sup\/main\/m1/ });
    expect(box).toBeChecked();
    // 待发布默认不勾选
    expect(screen.getByRole("checkbox", { name: /sup\/bk\/b1/ })).not.toBeChecked();
    fireEvent.click(box);
    expect(box).not.toBeChecked();
    // 取消（重载）后依然不勾选：排除记忆跨 load 生效
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    expect(screen.getByRole("checkbox", { name: /sup\/main\/m1/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /sup\/bk\/b1/ })).not.toBeChecked();
  });

  it("re-checking clears the persisted exclusion", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(persistPreview as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    const box = screen.getByRole("checkbox", { name: /sup\/main\/m1/ });
    fireEvent.click(box);
    expect(box).not.toBeChecked();
    fireEvent.click(box);
    expect(box).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    expect(screen.getByRole("checkbox", { name: /sup\/main\/m1/ })).toBeChecked();
  });
});

describe("GatewayPanel selection preview", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  it("sends source selections to backend preview instead of rebuilding providers", async () => {
    const preview = vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce({
        current: { "pi-switch-chat": { api: "openai-completions", models: [{ id: "chat-live" }] } },
        proposed: { "pi-switch-chat": { api: "openai-completions", models: [{ id: "chat-live" }, { id: "chat-new" }] } },
        conflicts: [],
        pending_count: 1,
        diff: { added: ["pi-switch-chat/chat-new"], removed: [], changed: [] },
        groups: [{ supplier: "deepseek", channel: "main", gatewayProvider: "pi-switch-chat", models: [
          { id: "chat-live", status: "published" },
          { id: "chat-new", status: "pending" },
        ] }],
        removed: [],
      } as any)
      .mockResolvedValue({
        current: { "pi-switch-chat": { api: "openai-completions", models: [{ id: "chat-live" }] } },
        proposed: { "pi-switch-chat": { api: "openai-completions", models: [{ id: "chat-new" }] } },
        conflicts: [],
        pending_count: 1,
        diff: { added: ["pi-switch-chat/chat-new"], removed: ["pi-switch-chat/chat-live"], changed: [] },
        groups: [{ supplier: "deepseek", channel: "main", gatewayProvider: "pi-switch-chat", models: [{ id: "chat-new", status: "pending" }] }],
        removed: [],
      } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByRole("checkbox", { name: /deepseek\/main\/chat-new/ })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("checkbox", { name: /deepseek\/main\/chat-new/ }));
    await waitFor(() => expect(preview).toHaveBeenLastCalledWith(expect.objectContaining({
      selected: [
        { supplier: "deepseek", channel: "main", model: "chat-live" },
        { supplier: "deepseek", channel: "main", model: "chat-new" },
      ],
    })));
  });
});

describe("GatewayPanel fixed provider projection", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  it("shows target providers with source channels and publishes fixed keys", async () => {
    const fixedPreview = {
      current: {
        "pi-switch-chat": {
          api: "openai-completions",
          baseUrl: "http://127.0.0.1:43112/v1",
          models: [{ id: "chat-live" }],
          proxy: false,
        },
        "pi-switch-res": {
          api: "openai-responses",
          baseUrl: "http://127.0.0.1:43112/v1",
          models: [{ id: "res-live" }],
          proxy: false,
        },
      },
      proposed: {
        "pi-switch-chat": {
          api: "openai-completions",
          baseUrl: "http://127.0.0.1:43112/v1",
          models: [{ id: "chat-live" }, { id: "chat-new" }],
          proxy: false,
        },
        "pi-switch-res": {
          api: "openai-responses",
          baseUrl: "http://127.0.0.1:43112/v1",
          models: [{ id: "res-live" }],
          proxy: false,
        },
		},
		diff: { added: ["pi-switch-chat/chat-new"], removed: [], changed: [] },
      conflicts: [],
      pending_count: 1,
      groups: [
        {
          supplier: "deepseek",
          channel: "main",
          gatewayProvider: "pi-switch-chat",
          models: [
            { id: "chat-live", status: "published" },
            { id: "chat-new", status: "pending" },
          ],
        },
        {
          supplier: "oc",
          channel: "responses",
          gatewayProvider: "pi-switch-res",
          models: [{ id: "res-live", status: "published" }],
        },
      ],
      removed: [],
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(fixedPreview as any);
const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
vi.spyOn(api, "getState").mockResolvedValue({ settings: { proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    renderGateway();
    await waitFor(() => expect(screen.getByText("pi-switch-chat · deepseek / main")).toBeInTheDocument());
    expect(screen.getByText("pi-switch-res · oc / responses")).toBeInTheDocument();
		const pending = screen.getByRole("checkbox", { name: "deepseek/main/chat-new" });
    expect(pending).not.toBeChecked();
    fireEvent.click(pending);
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const providers = payload.providers as Record<string, { models: Array<{ id: string }> }>;
    expect(Object.keys(providers).sort()).toEqual(["pi-switch-chat", "pi-switch-res"]);
    expect(providers["pi-switch-chat"].models.map((m) => m.id)).toContain("chat-new");
    expect(providers["pi-switch-res"].models.map((m) => m.id)).toEqual(["res-live"]);
    expect(providers["deepseek/main"]).toBeUndefined();
    expect(providers["oc/responses"]).toBeUndefined();
  });
});

describe("GatewayPanel post-publish selection", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  it("keeps the selected subset in the post-publish draft and shows one success toast", async () => {
    const gateway = (ids: string[]) => ({
      "pi-switch-chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        apiKey: "pi-switch-proxy",
        models: ids.map((id) => ({ id })),
        proxy: false,
      },
    });
    const full = gateway(["model-a", "model-b"]);
    const selected = gateway(["model-a"]);
    const candidateGroup = {
      supplier: "sup",
      channel: "main",
      gatewayProvider: "pi-switch-chat",
      models: ["model-a", "model-b"].map((id) => ({ id, status: "pending" })),
    };
    const response = (
      current: Record<string, unknown>,
      proposed: Record<string, unknown>,
      pending_count: number,
      statuses: string[],
    ) => backendPreview(current, proposed, {
      pending_count,
      groups: [{
        ...candidateGroup,
        models: ["model-a", "model-b"].map((id, i) => ({ id, status: statuses[i] })),
      }],
    }) as any;
    const preview = vi.spyOn(api, "previewGateway")
      .mockResolvedValueOnce(response({}, full, 2, ["pending", "pending"]))
      .mockResolvedValueOnce(response({}, selected, 1, ["pending", "pending"]))
      .mockResolvedValueOnce(response({}, selected, 1, ["pending", "pending"]))
      .mockImplementation(async (input) => input?.selected !== undefined
        ? response(selected, selected, 0, ["published", "pending"])
        : response(selected, full, 1, ["published", "pending"]));
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);

    renderGateway();
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "sup/main/model-a" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("checkbox", { name: "sup/main/model-a" }));
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));

    await waitFor(() => expect(apply).toHaveBeenCalledWith({ providers: selected }));
    await waitFor(() => {
      const value = JSON.parse((screen.getByLabelText("gateway json") as HTMLTextAreaElement).value);
      expect(value.providers["pi-switch-chat"].models.map((model: { id: string }) => model.id)).toEqual(["model-a"]);
    });
    expect(screen.getAllByText("Saved")).toHaveLength(1);
  });
});

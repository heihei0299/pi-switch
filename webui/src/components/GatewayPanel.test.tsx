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
    // status bar shows a newly added bare model under the channel provider
    expect(screen.getByText(/\+1 added/)).toBeInTheDocument();
    expect(screen.getByText(/-0 removed/)).toBeInTheDocument();
    expect(screen.getByText(/~0 changed/)).toBeInTheDocument();
    expect(screen.getByText(/待发布数: 1/)).toBeInTheDocument();
    expect(screen.getByText(/上次发布时间/)).toBeInTheDocument();
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


  it("shows conflicts when preview returns conflicts", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue({ current: currentGw, proposed: { ...proposedGw, baseUrl: "http://127.0.0.1:43113/v1" }, conflicts: ["baseUrl", "models"] } as any);
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

  it("subset pending follows the selection", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    // 默认只勾选已发布：子集与已注入一致 → 0
    await waitFor(() => expect(screen.getByText(/勾选子集待发布：0/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("checkbox", { name: /sup\/bk\/b1/ }));
    // 勾选 b1：子集多出待发布 → 1
    await waitFor(() => expect(screen.getByText(/勾选子集待发布：1/)).toBeInTheDocument());
  });
});

describe("GatewayPanel gateway id-shape + delete", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  // Preview returns the inner per-channel provider map with bare model ids.
  const mixedPreview = {
    current: {
      "oc/chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [
          { id: "mimo-v2.5" },
          { id: "muse-spark-1.3-contributor" },
        ],
        proxy: false,
      },
    },
    proposed: {
      "oc/chat": {
        api: "openai-completions",
        baseUrl: "http://127.0.0.1:43112/v1",
        models: [
          { id: "mimo-v2.5" },
          { id: "muse-spark-1.3-contributor" },
          { id: "omen-alpha" },
        ],
        proxy: false,
      },
    },
    conflicts: [],
    pending_count: 1,
    groups: [],
    removed: [],
  };

  function mockApply() {
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "getState").mockResolvedValue({ settings: { gatewayApi: "openai-completions", proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    return apply;
  }

  it("shows and publishes bare ids under the channel provider", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(mixedPreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    // 新裸 id 形态：输入框直接显示裸 id
    const idInputs = screen.getAllByLabelText("Model ID") as HTMLInputElement[];
    const values = idInputs.map((el) => el.value);
    expect(values).toContain("mimo-v2.5");
    expect(values).toContain("muse-spark-1.3-contributor");
    // 不应再有三段式 id 在输入框
    expect(values).not.toContain("oc/chat/mimo-v2.5");
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const providers = payload.providers as Record<string, { models: Array<{ id: string }> }>;
    const allIds: string[] = [];
    for (const prov of Object.values(providers ?? {})) {
      for (const m of prov.models) allIds.push(m.id);
    }
    // 发布载荷为裸 id 按渠道分 provider
    expect(allIds).toContain("mimo-v2.5");
    expect(allIds).toContain("muse-spark-1.3-contributor");
    expect(allIds).not.toContain("oc/mimo-v2.5");
    expect(allIds).not.toContain("omen-alpha");
  });

  it("deleting a draft row removes it from the apply payload", async () => {
    const singlePreview = {
      current: channelGateway([{ id: "m1" }]),
      proposed: channelGateway([{ id: "m1" }]),
      conflicts: [],
      pending_count: 0,
      groups: [],
      removed: [],
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(singlePreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect((screen.getByLabelText("Model ID") as HTMLInputElement).value).toBe("m1");
    fireEvent.click(screen.getByRole("button", { name: "remove" }));
    await waitFor(() => expect(screen.queryByLabelText("Model ID")).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const providers = payload.providers as Record<string, { models: Array<{ id: string }> }>;
    const allIds: string[] = [];
    for (const prov of Object.values(providers ?? {})) {
      for (const m of prov.models) allIds.push(m.id);
    }
    expect(allIds).not.toContain("m1");
    expect(allIds).toHaveLength(0);
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

describe("GatewayPanel display-to-full id mapping", () => {
  beforeEach(() => {
    window.localStorage?.clear();
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    window.localStorage?.clear();
  });

  const editPreview = {
    current: channelGateway([{ id: "mimo-v2.5" }]),
    proposed: channelGateway([{ id: "mimo-v2.5" }, { id: "omen-alpha" }]),
    conflicts: [],
    pending_count: 1,
    groups: [],
    removed: [],
  };

  function mockApply() {
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "getState").mockResolvedValue({ settings: { gatewayApi: "openai-completions", proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    return apply;
  }

  async function applyIds() {
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const input = screen.getByLabelText("Model ID") as HTMLInputElement;
    expect(input.value).toBe("mimo-v2.5");
    return { apply, input };
  }

  it("retyping the same short name keeps the mapped full id", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(editPreview as any);
    const { apply, input } = await applyIds();
    fireEvent.change(input, { target: { value: "mimo-v2.5" } });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = ((payload.providers as Record<string, { models: Array<{ id: string }> }>)?.["oc/chat"]?.models ?? payload.models ?? []) as Array<{ id: string }>;
    const flatIds = (Array.isArray(ids) ? ids : []).map((m: any) => m.id ?? m);
    expect(flatIds).toContain("mimo-v2.5");
  });

  it("pasting a known full id switches the mapping", async () => {
    const stalePreview = {
      current: channelGateway([{ id: "old-model" }]),
      proposed: channelGateway([{ id: "omen-alpha" }]),
      conflicts: [],
      pending_count: 1,
      groups: [],
      removed: ["old-model"],
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(stalePreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const input = screen.getByLabelText("Model ID") as HTMLInputElement;
    expect(input.value).toBe("old-model");
    fireEvent.change(input, { target: { value: "omen-alpha" } });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const models = (payload.providers?.["oc/chat"]?.models ?? payload.models) as Array<{ id: string }>;
    const ids = (models ?? []).map((m: any) => m.id);
    expect(ids).toContain("omen-alpha");
  });

  it("typing a brand-new id opts it in for publish", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(editPreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const input = screen.getByLabelText("Model ID") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "custom-new" } });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const models = (payload.providers?.["oc/chat"]?.models ?? payload.models) as Array<{ id: string }>;
    const ids = (models ?? []).map((m: any) => m.id);
    // 手工输入是显式意图：新 id 自动纳入本次发布
    expect(ids).toContain("custom-new");
  });
});

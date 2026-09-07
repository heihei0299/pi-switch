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
    current: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "sup/main/m1" }], proxy: false },
    proposed: {
      api: "openai-completions",
      baseUrl: "http://127.0.0.1:43112/v1",
      models: [{ id: "sup/main/m1" }, { id: "sup/bk/b1" }],
      proxy: false,
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

  it("unchecking a candidate excludes it from the apply payload", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "getState").mockResolvedValue({ settings: { gatewayApi: "openai-completions", proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    const box = screen.getByRole("checkbox", { name: /sup\/bk\/b1/ });
    expect(box).toBeChecked();
    fireEvent.click(box);
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = (payload.models as Array<{ id: string }>).map((m) => m.id);
    expect(ids).toContain("sup/main/m1");
    expect(ids).not.toContain("sup/bk/b1");
    expect(ids).not.toContain("ghost/x");
  });

  it("subset pending follows the selection", async () => {
    renderGrouped();
    await waitFor(() => expect(screen.getByText("sup / bk")).toBeInTheDocument());
    // 全选：b1 待发布 → 子集待发布 1
    expect(screen.getByText(/勾选子集待发布：1/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox", { name: /sup\/bk\/b1/ }));
    // 取消 b1：子集与已注入一致 → 0
    await waitFor(() => expect(screen.getByText(/勾选子集待发布：0/)).toBeInTheDocument());
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

  // current 混入历史短 id + 三段式 id：strip channel 后全部撞成 oc/mimo-v2.5
  const mixedPreview = {
    current: {
      api: "openai-completions",
      baseUrl: "http://127.0.0.1:43112/v1",
      models: [
        { id: "oc/mimo-v2.5" },
        { id: "oc/chat/mimo-v2.5" },
        { id: "oc/responses/muse-spark-1.3-contributor" },
      ],
      proxy: false,
    },
    proposed: {
      api: "openai-completions",
      baseUrl: "http://127.0.0.1:43112/v1",
      models: [
        { id: "oc/chat/mimo-v2.5" },
        { id: "oc/responses/muse-spark-1.3-contributor" },
        { id: "oc/chat/omen-alpha" },
      ],
      proxy: false,
    },
    conflicts: [],
    pending_count: 2,
    groups: [],
    removed: ["oc/mimo-v2.5"],
  };

  function mockApply() {
    const apply = vi.spyOn(api, "applyGateway").mockResolvedValue({ ok: true } as any);
    vi.spyOn(api, "getState").mockResolvedValue({ settings: { gatewayApi: "openai-completions", proxy: { host: "127.0.0.1", port: 43112 } } } as any);
    return apply;
  }

  it("shows short display ids but publishes full three-segment ids", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(mixedPreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    // 输入框只显示短名：oc/chat/mimo-v2.5 与历史短 id oc/mimo-v2.5 显示一致但数据不撞车
    const idInputs = screen.getAllByLabelText("Model ID") as HTMLInputElement[];
    const values = idInputs.map((el) => el.value);
    expect(values).toContain("oc/mimo-v2.5");
    expect(values).toContain("oc/muse-spark-1.3-contributor");
    expect(values).not.toContain("oc/chat/mimo-v2.5");
    // 完整三段式 id 以标注形式保留在行内（title 锚定，不与 JSON 预览文本混淆）
    expect(screen.getAllByTitle("oc/chat/mimo-v2.5").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = (payload.models as Array<{ id: string }>).map((m) => m.id);
    // 发布载荷保持全限定形态，不再把剥离后的短 id 写回网关
    expect(ids).toContain("oc/chat/mimo-v2.5");
    expect(ids).toContain("oc/responses/muse-spark-1.3-contributor");
    expect(ids).toContain("oc/chat/omen-alpha");
    // 历史短 id 默认排除（后端 removed），不再复活
    expect(ids).not.toContain("oc/mimo-v2.5");
  });

  it("deleting a draft row removes it from the apply payload", async () => {
    const singlePreview = {
      current: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "sup/main/m1" }], proxy: false },
      proposed: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "sup/main/m1" }], proxy: false },
      conflicts: [],
      pending_count: 0,
      groups: [],
      removed: [],
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(singlePreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    expect((screen.getByLabelText("Model ID") as HTMLInputElement).value).toBe("sup/m1");
    fireEvent.click(screen.getByRole("button", { name: "remove" }));
    await waitFor(() => expect(screen.queryByLabelText("Model ID")).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = (payload.models as Array<{ id: string }>).map((m) => m.id);
    expect(ids).not.toContain("sup/main/m1");
    expect(ids).toHaveLength(0);
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
    current: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "oc/chat/mimo-v2.5" }], proxy: false },
    proposed: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "oc/chat/mimo-v2.5" }, { id: "oc/chat/omen-alpha" }], proxy: false },
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
    expect(input.value).toBe("oc/mimo-v2.5");
    return { apply, input };
  }

  it("retyping the same short name keeps the mapped full id", async () => {
    vi.spyOn(api, "previewGateway").mockResolvedValue(editPreview as any);
    const { apply, input } = await applyIds();
    fireEvent.change(input, { target: { value: "oc/mimo-v2.5" } });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = (payload.models as Array<{ id: string }>).map((m) => m.id);
    expect(ids).toContain("oc/chat/mimo-v2.5");
    expect(ids).not.toContain("oc/mimo-v2.5");
  });

  it("pasting a known full id switches the mapping", async () => {
    const stalePreview = {
      current: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "oc/chat/old-model" }], proxy: false },
      proposed: { api: "openai-completions", baseUrl: "http://127.0.0.1:43112/v1", models: [{ id: "oc/chat/omen-alpha" }], proxy: false },
      conflicts: [],
      pending_count: 1,
      groups: [],
      removed: ["oc/chat/old-model"],
    };
    vi.spyOn(api, "previewGateway").mockResolvedValue(stalePreview as any);
    const apply = mockApply();
    renderGateway();
    await waitFor(() => expect(screen.getByText(/Current vs Proposed/)).toBeInTheDocument());
    const input = screen.getByLabelText("Model ID") as HTMLInputElement;
    expect(input.value).toBe("oc/old-model");
    fireEvent.change(input, { target: { value: "oc/chat/omen-alpha" } });
    fireEvent.click(screen.getByRole("button", { name: "应用到 Pi" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    const payload = apply.mock.calls[0][0] as any;
    const ids = (payload.models as Array<{ id: string }>).map((m) => m.id);
    expect(ids).toEqual(["oc/chat/omen-alpha"]);
  });
});

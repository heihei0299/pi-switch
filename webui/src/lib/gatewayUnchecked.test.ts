import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  UNCHECKED_KEY,
  addUncheckedId,
  loadUncheckedIds,
  removeUncheckedId,
} from "./gatewayUnchecked";

// 本仓库 vitest 未提供 localStorage（既有测试全用 ?. 容错），此处装内存桩。
function ensureStorage() {
  if (!window.localStorage) {
    const store = new Map<string, string>();
    Object.defineProperty(window, "localStorage", {
      value: {
        getItem: (k: string) => (store.has(k) ? store.get(k) as string : null),
        setItem: (k: string, v: string) => { store.set(k, String(v)); },
        removeItem: (k: string) => { store.delete(k); },
        clear: () => store.clear(),
      },
      configurable: true,
    });
  }
}

function seed(ids: string[]) {
  window.localStorage.setItem(UNCHECKED_KEY, JSON.stringify(ids));
}
describe("gatewayUnchecked", () => {
  beforeEach(() => {
    ensureStorage();
    window.localStorage.clear();
  });
  it("loads an empty set when nothing is stored", () => {
    window.localStorage.clear();
    expect(loadUncheckedIds()).toEqual(new Set());
  });
  it("round-trips unchecked ids and prunes stale ones against known ids", () => {
    window.localStorage.clear();
    addUncheckedId("sup/bk/b1");
    addUncheckedId("ghost/x");
    expect(loadUncheckedIds()).toEqual(new Set(["sup/bk/b1", "ghost/x"]));
    // 修剪：只保留仍在候选/草稿里的 id，并写回
    expect(loadUncheckedIds(new Set(["sup/bk/b1"]))).toEqual(new Set(["sup/bk/b1"]));
    expect(JSON.parse(window.localStorage.getItem(UNCHECKED_KEY) as string)).toEqual([
      "sup/bk/b1",
    ]);
    removeUncheckedId("sup/bk/b1");
    expect(loadUncheckedIds()).toEqual(new Set());
  });
  it("tolerates corrupted storage", () => {
    window.localStorage.setItem(UNCHECKED_KEY, "{not-json");
    expect(loadUncheckedIds()).toEqual(new Set());
    window.localStorage.setItem(UNCHECKED_KEY, JSON.stringify({ a: 1 }));
    expect(loadUncheckedIds()).toEqual(new Set());
  });
  it("add is idempotent", () => {
    window.localStorage.clear();
    addUncheckedId("sup/bk/b1");
    addUncheckedId("sup/bk/b1");
    expect(loadUncheckedIds()).toEqual(new Set(["sup/bk/b1"]));
    expect(vi).toBeDefined();
  });
});

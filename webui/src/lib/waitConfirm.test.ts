import { describe, expect, it, vi } from "vitest";
import { saveWithConfirm } from "./waitConfirm";

describe("S2 optimistic update waiting confirm", () => {
  it("waits for GET /api/profiles success before updating local", async () => {
    const order: string[] = [];
    const put = vi.fn(async () => { order.push("put"); });
    const getProfiles = vi.fn(async () => { order.push("get"); return { profiles: { a: {} } } as any; });
    const onSuccess = vi.fn((data) => { order.push("success"); expect(data.profiles).toBeDefined(); });
    await saveWithConfirm(put, getProfiles, onSuccess, vi.fn());
    expect(order).toEqual(["put", "get", "success"]);
    expect(put).toHaveBeenCalledTimes(1);
    expect(getProfiles).toHaveBeenCalledTimes(1);
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("does not update local if GET fails (avoid race with gateway publish)", async () => {
    const put = vi.fn(async () => {});
    const getProfiles = vi.fn(async () => { throw new Error("GET failed"); });
    const onSuccess = vi.fn();
    const onError = vi.fn();
    await expect(saveWithConfirm(put, getProfiles, onSuccess, onError)).rejects.toThrow("GET failed");
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled(); // saveWithConfirm rethrows; caller decides
  });

  it("does not call GET if PUT fails", async () => {
    const put = vi.fn(async () => { throw new Error("PUT failed"); });
    const getProfiles = vi.fn(async () => ({ profiles: {} } as any));
    const onSuccess = vi.fn();
    await expect(saveWithConfirm(put, getProfiles, onSuccess, vi.fn())).rejects.toThrow("PUT failed");
    expect(getProfiles).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
  });
});

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { api } from "./api";

describe("api.getCredits", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("calls GET /api/profiles/:name/credits and returns normalized data", async () => {
    const fake = { balance: 10, used: 20, total: 30, remaining: 10, percent: 66, resetAt: "2026-09-01", expiry: "2026-09-01", raw: {} };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      text: async () => JSON.stringify(fake),
    } as any);

    const res = await (api as any).getCredits("my-profile");

    expect(fetchSpy).toHaveBeenCalledWith("/api/profiles/my-profile/credits", expect.objectContaining({ method: "GET" }));
    expect(res.balance).toBe(10);
    expect(res.used).toBe(20);
    expect(res.total).toBe(30);
  });

  it("throws with error message when upstream returns error", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      text: async () => JSON.stringify({ error: "upstream 401: bad key" }),
    } as any);

    await expect((api as any).getCredits("bad")).rejects.toThrow("upstream 401");
  });

  it("encodes profile name", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      text: async () => JSON.stringify({ balance: 1, used: 0, total: 1, remaining: 1, percent: 0, raw: {} }),
    } as any);
    await (api as any).getCredits("a/b c");
    expect(fetchSpy).toHaveBeenCalledWith("/api/profiles/a%2Fb%20c/credits", expect.any(Object));
  });
});

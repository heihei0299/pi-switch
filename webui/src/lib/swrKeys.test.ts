import { describe, expect, it, vi, beforeEach } from "vitest";
vi.mock("swr", () => ({ mutate: vi.fn(() => Promise.resolve()) }));
import { SWR_KEY_PROFILES, SWR_KEY_GATEWAY, swrKeyStats, PUT_MUTATE_KEYS } from "./swrKeys";
import { mutateAfterProfilePut, mutateAfterGatewayPublish } from "../store/swr";
import { mutate } from "swr";

describe("S1 SWR three keys with mutate", () => {
  beforeEach(() => vi.clearAllMocks());

  it("defines profiles and gateway keys", () => {
    expect(SWR_KEY_PROFILES).toEqual(["profiles"]);
    expect(SWR_KEY_GATEWAY).toEqual(["gateway"]);
  });

  it("stats key includes window", () => {
    expect(swrKeyStats("today", 1, 2)).toEqual(["stats", "today:1:2"]);
    expect(swrKeyStats("custom", 100, 200)).toEqual(["stats", "custom:100:200"]);
  });

  it("PUT mutate keys include both profiles and gateway", () => {
    expect(PUT_MUTATE_KEYS).toEqual([["profiles"], ["gateway"]]);
  });

  it("mutateAfterProfilePut triggers mutate for both keys", async () => {
    await mutateAfterProfilePut();
    expect(mutate).toHaveBeenCalledWith(["profiles"]);
    expect(mutate).toHaveBeenCalledWith(["gateway"]);
  });

  it("mutateAfterGatewayPublish triggers mutate for both keys", async () => {
    await mutateAfterGatewayPublish();
    expect(mutate).toHaveBeenCalledWith(["profiles"]);
    expect(mutate).toHaveBeenCalledWith(["gateway"]);
  });

});

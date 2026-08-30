import { describe, expect, it } from "vitest";
import { isCreditsSupported } from "./credits";
import type { ProviderProfile } from "../types";

function profile(overrides: Partial<ProviderProfile>): ProviderProfile {
  return {
    api: "openai-completions",
    baseUrl: "",
    apiKey: "",
    models: [],
    proxy: false,
    ...overrides,
  } as ProviderProfile;
}

describe("isCreditsSupported", () => {
  it("returns true when primary baseUrl contains opencode.ai case-insensitive", () => {
    expect(isCreditsSupported(profile({ baseUrl: "https://api.opencode.ai/v1" }))).toBe(true);
    expect(isCreditsSupported(profile({ baseUrl: "https://API.OPENCODE.AI/v1" }))).toBe(true);
    expect(isCreditsSupported(profile({ baseUrl: "https://api.opencode.ai" }))).toBe(true);
  });

  it("returns false when baseUrl does not contain opencode.ai", () => {
    expect(isCreditsSupported(profile({ baseUrl: "https://api.example.com/v1" }))).toBe(false);
    expect(isCreditsSupported(profile({ baseUrl: "" }))).toBe(false);
  });

  it("checks primary upstream only when multiple upstreams", () => {
    // primary hit → true even if secondary not
    expect(
      isCreditsSupported(
        profile({
          baseUrl: "https://ignore.com",
          upstreams: [
            { baseUrl: "https://api.opencode.ai/v1", apiKey: "k1" },
            { baseUrl: "https://other.com/v1", apiKey: "k2" },
          ],
        }),
      ),
    ).toBe(true);
    // primary miss → false even if secondary hit
    expect(
      isCreditsSupported(
        profile({
          upstreams: [
            { baseUrl: "https://other.com/v1", apiKey: "k1" },
            { baseUrl: "https://api.opencode.ai/v1", apiKey: "k2" },
          ],
        }),
      ),
    ).toBe(false);
  });

  it("falls back to baseUrl when no upstreams", () => {
    expect(
      isCreditsSupported(
        profile({
          baseUrl: "https://api.opencode.ai/v1",
          upstreams: [],
        }),
      ),
    ).toBe(true);
  });
});

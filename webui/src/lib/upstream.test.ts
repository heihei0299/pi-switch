import { describe, expect, it } from "vitest";
import { hasUpstreams, resolvedUpstreams, type ProviderProfile } from "../types";

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

describe("upstream fallback (has_upstreams / resolved_upstreams)", () => {
  it("hasUpstreams false when upstreams empty or missing", () => {
    expect(hasUpstreams(profile({}))).toBe(false);
    expect(hasUpstreams(profile({ upstreams: [] }))).toBe(false);
    expect(hasUpstreams(profile({ baseUrl: "http://a/v1" }))).toBe(false);
  });

  it("hasUpstreams true when upstreams non-empty", () => {
    expect(hasUpstreams(profile({ upstreams: [{ baseUrl: "http://a/v1", apiKey: "k" }] }))).toBe(true);
  });

  it("resolvedUpstreams fallback to single baseUrl/apiKey when no upstreams", () => {
    const p = profile({ baseUrl: "http://a/v1", apiKey: "sk-1", headers: { "x-a": "1" } });
    const ups = resolvedUpstreams(p);
    expect(ups).toHaveLength(1);
    expect(ups[0].baseUrl).toBe("http://a/v1");
    expect(ups[0].apiKey).toBe("sk-1");
    expect(ups[0].headers).toEqual({ "x-a": "1" });
  });

  it("resolvedUpstreams returns upstreams directly when present (ignores single)", () => {
    const p = profile({
      baseUrl: "http://should-ignore/v1",
      apiKey: "ignore",
      upstreams: [
        { baseUrl: "http://a/v1", apiKey: "k1", weight: 2, name: "a" },
        { baseUrl: "http://b/v1", apiKey: "k2" },
      ],
    });
    const ups = resolvedUpstreams(p);
    expect(ups).toHaveLength(2);
    expect(ups[0].baseUrl).toBe("http://a/v1");
    expect(ups[0].weight).toBe(2);
    expect(ups[1].baseUrl).toBe("http://b/v1");
  });

  it("resolvedUpstreams empty when both single and upstreams empty", () => {
    expect(resolvedUpstreams(profile({}))).toHaveLength(0);
  });

  it("single field compatibility: weight/name not set fallback", () => {
    const p = profile({ upstreams: [{ baseUrl: "http://a/v1", apiKey: "k1" }] });
    const ups = resolvedUpstreams(p);
    expect(ups[0].weight).toBeUndefined();
    expect(ups[0].name).toBeUndefined();
  });
});

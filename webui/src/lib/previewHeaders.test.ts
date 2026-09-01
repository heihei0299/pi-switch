import { describe, expect, it } from "vitest";
import { mergePreviewHeaders, previewHeadersForUpstream } from "./previewHeaders";

describe("S3 userAgent debounce previewHeaders X-Custom merge", () => {
  it("merges profile and upstream headers, upstream overrides", () => {
    const profile = { "X-Custom": "from-profile", "X-Other": "1" };
    const upstream = { "X-Custom": "from-upstream", "X-New": "2" };
    const merged = mergePreviewHeaders(profile, upstream);
    expect(merged["X-Custom"]).toBe("from-upstream");
    expect(merged["X-Other"]).toBe("1");
    expect(merged["X-New"]).toBe("2");
  });

  it("upstream aggregation visible via resolvedUpstreams (upstream.headers > profile.headers)", () => {
    const profileHeaders = { Authorization: "Bearer profile" };
    const upsHeaders = { Authorization: "Bearer upstream", "X-Custom": "value" };
    const merged = previewHeadersForUpstream(profileHeaders, [{ headers: upsHeaders } as any], 0);
    expect(merged["Authorization"]).toBe("Bearer upstream");
    expect(merged["X-Custom"]).toBe("value");
  });

  it("does not mutate original headers objects (not auto write)", () => {
    const profile = { "X-Custom": "a" };
    const upstream = { "X-Custom": "b" };
    const beforeProfile = { ...profile };
    const beforeUpstream = { ...upstream };
    mergePreviewHeaders(profile, upstream);
    expect(profile).toEqual(beforeProfile);
    expect(upstream).toEqual(beforeUpstream);
  });

  it("returns empty when both missing", () => {
    expect(mergePreviewHeaders(undefined, undefined)).toEqual({});
    expect(mergePreviewHeaders({}, {})).toEqual({});
  });
});

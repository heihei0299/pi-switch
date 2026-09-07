import { describe, expect, it } from "vitest";
import { validateGatewayJson } from "./gatewayDiff";

describe("fixed gateway validation", () => {
  it("rejects a third provider using the pi-switch proxy identity", () => {
    const value = {
      providers: {
        "pi-switch-extra": {
          api: "openai-completions",
          baseUrl: "http://127.0.0.1:43112/v1",
          apiKey: "pi-switch-proxy",
          models: [],
        },
      },
    };
    const result = validateGatewayJson(JSON.stringify(value));
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/pi-switch-extra/);
  });

  it("requires the fixed API contract for fixed providers", () => {
    const value = {
      providers: {
        "pi-switch-res": {
          api: "openai-completions",
          baseUrl: "http://127.0.0.1:43112/v1",
          models: [],
        },
      },
    };
    const result = validateGatewayJson(JSON.stringify(value));
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/openai-responses/);
  });
});

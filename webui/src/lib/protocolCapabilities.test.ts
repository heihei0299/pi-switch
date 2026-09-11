import { describe, expect, it } from "vitest";
import {
  FALLBACK_PROTOCOL_APIS,
  allowedResponsesModes,
  defaultResponsesModeFor,
  protocolApiIds,
} from "./protocolCapabilities";

describe("protocol capability fixture", () => {
  it("exposes the canonical api set", () => {
    expect(protocolApiIds(FALLBACK_PROTOCOL_APIS)).toEqual([
      "openai-completions",
      "openai-responses",
      "anthropic-messages",
      "google-generative-ai",
    ]);
  });

  it("derives modes from the capability instead of a local rule", () => {
    expect(allowedResponsesModes(FALLBACK_PROTOCOL_APIS, "openai-responses")).toEqual(["auto", "passthrough"]);
    expect(allowedResponsesModes(FALLBACK_PROTOCOL_APIS, "google-generative-ai")).toEqual(["auto"]);
    expect(defaultResponsesModeFor(FALLBACK_PROTOCOL_APIS, "openai-completions")).toBe("convert");
  });
});

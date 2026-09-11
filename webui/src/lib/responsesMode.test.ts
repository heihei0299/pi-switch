import { describe, expect, it } from "vitest";
import type { ProtocolApiCapability } from "../types";
import { effectiveResponsesMode, responsesModeError } from "./responsesMode";
import { FALLBACK_PROTOCOL_APIS } from "./protocolCapabilities";

describe("responsesMode follows the backend capability", () => {
  it("uses the fixture fallback", () => {
    expect(effectiveResponsesMode({ api: "openai-responses", responsesMode: "auto" }, FALLBACK_PROTOCOL_APIS)).toBe("passthrough");
    expect(effectiveResponsesMode({ api: "openai-completions", responsesMode: "auto" }, FALLBACK_PROTOCOL_APIS)).toBe("convert");
    expect(responsesModeError("openai-completions", "convert", FALLBACK_PROTOCOL_APIS)).toBeNull();
    expect(responsesModeError("anthropic-messages", "convert", FALLBACK_PROTOCOL_APIS)).toBe("convert requires openai-completions");
  });

  it("follows a capability set that differs from the fallback", () => {
    const caps: ProtocolApiCapability[] = [
      { id: "openai-completions", label: "Chat", defaultMode: "auto", responsesModes: ["auto"], canProxy: true, canGateway: true },
    ];
    expect(effectiveResponsesMode({ api: "openai-completions", responsesMode: "auto" }, caps)).toBe("auto");
    expect(responsesModeError("openai-completions", "convert", caps)).toBe("convert requires openai-completions");
    expect(responsesModeError("openai-completions", "auto", caps)).toBeNull();
  });
});

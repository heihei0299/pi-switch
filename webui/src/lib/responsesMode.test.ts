import { afterEach, describe, expect, it } from "vitest";
import { effectiveResponsesMode, responsesModeError } from "./responsesMode";
import { setProtocolCapabilities } from "./protocolCapabilities";

afterEach(() => setProtocolCapabilities(undefined));

describe("responsesMode follows the backend capability", () => {
  it("uses the fallback rule before the backend answers", () => {
    expect(effectiveResponsesMode({ api: "openai-responses", responsesMode: "auto" })).toBe("passthrough");
    expect(effectiveResponsesMode({ api: "openai-completions", responsesMode: "auto" })).toBe("convert");
    expect(responsesModeError("openai-completions", "convert")).toBeNull();
    expect(responsesModeError("anthropic-messages", "convert")).toBe("convert requires openai-completions");
  });

  it("replaces the rule when the backend reports a different capability", () => {
    setProtocolCapabilities([
      { id: "openai-completions", label: "Chat", defaultMode: "auto", responsesModes: ["auto"], canProxy: true, canGateway: true },
    ]);
    // The backend says this api no longer derives convert and does not allow it.
    expect(effectiveResponsesMode({ api: "openai-completions", responsesMode: "auto" })).toBe("auto");
    expect(responsesModeError("openai-completions", "convert")).toBe("convert requires openai-completions");
    expect(responsesModeError("openai-completions", "auto")).toBeNull();
  });
});

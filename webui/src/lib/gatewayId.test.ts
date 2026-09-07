import { describe, expect, it } from "vitest";
import { resolveGatewayId, shortGatewayId } from "./gatewayId";

describe("shortGatewayId", () => {
  it("strips the channel segment from three-segment ids", () => {
    expect(shortGatewayId("oc/chat/mimo-v2.5")).toBe("oc/mimo-v2.5");
    expect(shortGatewayId("oc/responses/muse-spark-1.3-contributor")).toBe(
      "oc/muse-spark-1.3-contributor",
    );
  });
  it("leaves legacy short ids and bare ids untouched", () => {
    expect(shortGatewayId("oc/mimo-v2.5")).toBe("oc/mimo-v2.5");
    expect(shortGatewayId("deepseek/deepseek-v4-flash")).toBe("deepseek/deepseek-v4-flash");
    expect(shortGatewayId("mimo-v2.5")).toBe("mimo-v2.5");
    expect(shortGatewayId("")).toBe("");
  });
});

describe("resolveGatewayId", () => {
  const known = ["oc/chat/mimo-v2.5", "oc/responses/muse-spark-1.3-contributor"];
  it("keeps the full id when the display text is unchanged", () => {
    expect(resolveGatewayId("oc/mimo-v2.5", "oc/chat/mimo-v2.5", known)).toBe(
      "oc/chat/mimo-v2.5",
    );
  });
  it("accepts a pasted full id from the known set", () => {
    expect(
      resolveGatewayId("oc/responses/muse-spark-1.3-contributor", "oc/chat/mimo-v2.5", known),
    ).toBe("oc/responses/muse-spark-1.3-contributor");
  });
  it("passes custom or legacy short ids through untouched", () => {
    expect(resolveGatewayId("oc/my-custom", "oc/chat/mimo-v2.5", known)).toBe("oc/my-custom");
    expect(resolveGatewayId("oc/mimo-v2.5", "oc/mimo-v2.5", [])).toBe("oc/mimo-v2.5");
    expect(resolveGatewayId("", "", [])).toBe("");
  });
});

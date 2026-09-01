import { describe, expect, it } from "vitest";
import { formatCost } from "./format";

describe("S6 formatCost 0→$0.00, 0.0042→$0.0042, 12.34→$12.34, 1234→$1.2K, null→\"-\"", () => {
  it("renders exact mappings", () => {
    expect(formatCost(0)).toBe("$0.00");
    expect(formatCost(0.0042)).toBe("$0.0042");
    expect(formatCost(12.34)).toBe("$12.34");
    expect(formatCost(1234)).toBe("$1.2K");
    expect(formatCost(null)).toBe("-");
    expect(formatCost(undefined)).toBe("-");
  });
});

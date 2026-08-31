import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { JsonEditor } from "./JsonEditor";

describe("JsonEditor errorLine", () => {
  it("highlights error line with bg-red-500/10 text-red-400", () => {
    const { container } = render(<JsonEditor value={"{\n\"a\":1,\n\"b\":}"} onChange={() => {}} label="test" errorLine={3} />);
    const highlights = container.querySelectorAll(".bg-red-500\\/10");
    expect(highlights.length).toBeGreaterThan(0);
    // line number 3 should be red
    const lineNumbers = container.querySelectorAll('[aria-hidden="true"] div');
    // at least 3 lines
    const third = Array.from(container.querySelectorAll("div")).find((d) => d.textContent?.trim() === "3");
    expect(third).toBeTruthy();
    expect(third!.className).toContain("bg-red-500/10");
    expect(third!.className).toContain("text-red-400");
  });
  it("no highlight when null", () => {
    const { container } = render(<JsonEditor value={'{"a":1}'} onChange={() => {}} label="test" errorLine={null} />);
    expect(container.querySelectorAll(".bg-red-500\\/10").length).toBe(0);
  });
  it("no highlight when undefined", () => {
    const { container } = render(<JsonEditor value={'{"a":1}'} onChange={() => {}} label="test" />);
    expect(container.querySelectorAll(".bg-red-500\\/10").length).toBe(0);
  });
});

import { describe, expect, it } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { useDebounce } from "./useDebounce";

function DebounceDemo({ delay = 300 }: { delay?: number }) {
  const [value, setValue] = useState("initial");
  const debounced = useDebounce(value, delay);
  return (
    <div>
      <span data-testid="debounced">{debounced}</span>
      <button data-testid="btn-change" onClick={() => setValue("changed")}>change</button>
      <button data-testid="btn-change2" onClick={() => setValue("changed2")}>change2</button>
    </div>
  );
}

describe("S3 debounce 300ms", () => {
  it("debounces value by 300ms default", async () => {
    render(<DebounceDemo delay={300} />);
    expect(screen.getByTestId("debounced").textContent).toBe("initial");
    screen.getByTestId("btn-change").click();
    expect(screen.getByTestId("debounced").textContent).toBe("initial");
    await waitFor(() => expect(screen.getByTestId("debounced").textContent).toBe("changed"), { timeout: 1000 });
    cleanup();
  });

  it("resets timer on rapid changes (only last value after 300ms)", async () => {
    render(<DebounceDemo delay={300} />);
    screen.getByTestId("btn-change").click();
    // rapid second change before first debounce fires
    setTimeout(() => screen.getByTestId("btn-change2").click(), 100);
    // after 500ms total, should be changed2 not changed
    await waitFor(() => expect(screen.getByTestId("debounced").textContent).toBe("changed2"), { timeout: 1000 });
    cleanup();
  });
});

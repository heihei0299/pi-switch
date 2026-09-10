import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PackagesPanel } from "./PackagesPanel";
import { LanguageProvider } from "../i18n";
import { ToastProvider } from "./ui";
import { api } from "../api";

function renderPanel() {
  return render(
    <LanguageProvider configLang="en">
      <ToastProvider>
        <PackagesPanel refresh={vi.fn()} />
      </ToastProvider>
    </LanguageProvider>,
  );
}

describe("PackagesPanel Pi import", () => {
  beforeEach(() => {
    vi.spyOn(api, "getPackages").mockResolvedValue({ packages: [] });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("shows the backend import count and message", async () => {
    const imported = vi.spyOn(api, "importPackages").mockResolvedValue({
      ok: true,
      count: 2,
      discovered: 2,
      skipped: 0,
      status: "imported",
      message: "Imported 2 Pi packages",
    });
    renderPanel();
    await waitFor(() => expect(screen.getByRole("button", { name: /Import from Pi Agent/ })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Import from Pi Agent/ }));
    await waitFor(() => expect(imported).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText("Imported 2 Pi packages")).toBeInTheDocument());
  });

  it("does not present an empty discovery as a successful import", async () => {
    vi.spyOn(api, "importPackages").mockResolvedValue({
      ok: true,
      count: 0,
      discovered: 0,
      skipped: 1,
      status: "empty",
      message: "No valid Pi packages were discovered",
    });
    renderPanel();
    await waitFor(() => expect(screen.getByRole("button", { name: /Import from Pi Agent/ })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Import from Pi Agent/ }));
    await waitFor(() => expect(screen.getByText("No valid Pi packages were discovered")).toBeInTheDocument());
  });
});

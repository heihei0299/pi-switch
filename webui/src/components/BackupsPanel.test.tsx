import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { BackupsPanel } from "./BackupsPanel";
import { LanguageProvider } from "../i18n";

function renderPanel() {
  return render(
    <LanguageProvider configLang="en">
      <BackupsPanel />
    </LanguageProvider>,
  );
}

// Ticket 04: the config backup / encrypted export / import / restore APIs answer
// 501, so the panel must not offer controls that would report success for work
// that never happens.
describe("BackupsPanel honesty", () => {
  afterEach(() => cleanup());

  it("offers no interactive control at all, so no renamed button can slip back in", () => {
    const { container } = renderPanel();

    // Assert the property, not the labels: any button or form control here could
    // only drive an endpoint that answers 501.
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(container.querySelectorAll("input, textarea, select, form")).toHaveLength(0);
  });

  it("explains that the feature is unavailable instead of showing an empty list", () => {
    renderPanel();

    expect(screen.getByText(/not implemented yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/No backups yet/i)).toBeNull();
  });
});

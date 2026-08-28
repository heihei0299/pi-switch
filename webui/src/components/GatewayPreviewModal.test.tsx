import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GatewayPreviewModal } from "./GatewayPreviewModal";

const current = { api: "openai-completions", baseUrl: "http://a/v1", models: [], proxy: false };
const proposed = { api: "openai-completions", baseUrl: "http://b/v1", models: [{ id: "p/m" }], proxy: false };

afterEach(cleanup);

describe("GatewayPreviewModal", () => {
  it("renders diff and editable json", () => {
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={["baseUrl"]} onConfirm={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByText(/Current vs Proposed/i)).toBeInTheDocument();
    expect(screen.getByText(/proposed json/i)).toBeInTheDocument();
    expect(screen.getByDisplayValue(/http:\/\/b\/v1/)).toBeInTheDocument();
  });

  it("highlights conflicts", () => {
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={["baseUrl"]} onConfirm={vi.fn()} onClose={vi.fn()} />);
    // conflict key should be rendered with amber highlight
    expect(screen.getByText("baseUrl")).toBeInTheDocument();
  });

  it("disables confirm when json invalid", () => {
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={[]} onConfirm={vi.fn()} onClose={vi.fn()} />);
    const ta = screen.getByLabelText(/proposed json/i);
    fireEvent.change(ta, { target: { value: "{ invalid" } });
    const btn = screen.getByRole("button", { name: /confirm/i });
    expect(btn).toBeDisabled();
    expect(screen.getByText(/invalid json/i)).toBeInTheDocument();
  });

  it("calls onConfirm with parsed value when valid", () => {
    const onConfirm = vi.fn();
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={[]} onConfirm={onConfirm} onClose={vi.fn()} />);
    const btn = screen.getByRole("button", { name: /confirm/i });
    fireEvent.click(btn);
    expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({ baseUrl: "http://b/v1" }));
  });

  it("edits json and confirms edited value", () => {
    const onConfirm = vi.fn();
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={[]} onConfirm={onConfirm} onClose={vi.fn()} />);
    const ta = screen.getByLabelText(/proposed json/i);
    const edited = { ...proposed, baseUrl: "http://edited/v1" };
    fireEvent.change(ta, { target: { value: JSON.stringify(edited, null, 2) } });
    fireEvent.click(screen.getByRole("button", { name: /confirm/i }));
    expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({ baseUrl: "http://edited/v1" }));
  });

  it("calls onClose when cancel clicked", () => {
    const onClose = vi.fn();
    render(<GatewayPreviewModal current={current} proposed={proposed} conflicts={[]} onConfirm={vi.fn()} onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalled();
  });
});

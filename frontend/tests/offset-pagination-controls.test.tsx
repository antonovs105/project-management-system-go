import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { OffsetPaginationControls } from "../src/components/OffsetPaginationControls";

describe("OffsetPaginationControls", () => {
  it("navigates by the server-reported page size", () => {
    const onOffsetChange = vi.fn();
    render(<OffsetPaginationControls page={{ items: ["a", "b"], limit: 2, offset: 2, hasMore: true }} onOffsetChange={onOffsetChange} />);

    expect(screen.getByText("Page 2 · 2 shown")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(onOffsetChange).toHaveBeenNthCalledWith(1, 0);
    expect(onOffsetChange).toHaveBeenNthCalledWith(2, 4);
  });

  it("disables unavailable boundaries", () => {
    render(<OffsetPaginationControls page={{ items: [], limit: 25, offset: 0, hasMore: false }} onOffsetChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });
});

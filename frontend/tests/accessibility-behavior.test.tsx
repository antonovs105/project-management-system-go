import { fireEvent, render, screen } from "@testing-library/react";
import axe from "axe-core";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../src/components/I18nProvider";
import { Modal } from "../src/components/ui";
import { TicketBoard } from "../src/features/tickets/TicketBoard";
import type { BoardTicket } from "../src/types";

const ticket: BoardTicket = {
  id: "11111111-1111-4111-8111-111111111111",
  title: "Keyboard ticket",
  description: "",
  status: "open",
  priority: "medium",
  type: "task",
  assignee_id: null,
  due_date: null,
  label_ids: [],
  version: 1,
};

function ModalFixture() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Open settings</button>
      <Modal open={open} title="Settings" onClose={() => setOpen(false)}>
        <button type="button">First action</button>
        <button type="button">Last action</button>
      </Modal>
    </>
  );
}

describe("shared accessibility behavior", () => {
	it("passes automated axe checks for the interactive board", async () => {
		const { container } = render(
			<I18nProvider>
				<main>
					<h1>Project board</h1>
					<TicketBoard tickets={[ticket]} members={[]} onOpenTicket={vi.fn()} onMoveTicket={vi.fn()} />
				</main>
			</I18nProvider>,
		);

		const results = await axe.run(container, { rules: { "color-contrast": { enabled: false } } });
		expect(results.violations, results.violations.map((violation) => `${violation.id}: ${violation.help}`).join("\n")).toEqual([]);
	});

  it("labels independent ticket open and keyboard drag controls", () => {
	render(<I18nProvider><TicketBoard tickets={[ticket]} members={[]} onOpenTicket={vi.fn()} onMoveTicket={vi.fn()} /></I18nProvider>);

    expect(screen.getByRole("button", { name: "Open ticket Keyboard ticket" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Move ticket Keyboard ticket" })).toHaveAttribute("aria-roledescription", "sortable");
  });

  it("focuses, labels, closes, and restores focus for dialogs", () => {
    render(<I18nProvider><ModalFixture /></I18nProvider>);
    const trigger = screen.getByRole("button", { name: "Open settings" });
    trigger.focus();
    fireEvent.click(trigger);

    expect(screen.getByRole("dialog", { name: "Settings" })).toBeInTheDocument();
	expect(screen.getByRole("button", { name: "Close" })).toHaveFocus();
    fireEvent.keyDown(document, { key: "Escape" });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });
});

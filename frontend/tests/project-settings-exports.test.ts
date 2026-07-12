import { describe, expect, it } from "vitest";
import { safeFilePart, ticketsToCSV } from "../src/features/projects/projectSettingsExports";
import type { Ticket } from "../src/types";

describe("project settings exports", () => {
  it("creates safe filenames and neutralizes spreadsheet formulas", () => {
    const ticket: Ticket = {
      id: "11111111-1111-4111-8111-111111111111",
      ap_id: "https://example.test/tickets/1",
      title: "=HYPERLINK(\"https://attacker.test\")",
      description: "",
      status: "open",
      priority: "medium",
      type: "task",
      rank: "HZZZZZZZZZZZ",
      parent_id: null,
      project_id: "22222222-2222-4222-8222-222222222222",
      reporter_id: "33333333-3333-4333-8333-333333333333",
      assignee_id: null,
      is_resolved: false,
      due_date: null,
      label_ids: [],
      version: 1,
      archived_at: null,
      created_at: "2026-07-12T00:00:00Z",
      updated_at: "2026-07-12T00:00:00Z",
    };

    expect(safeFilePart("  Quarterly / Plan  ")).toBe("quarterly-plan");
    expect(ticketsToCSV([ticket])).toContain(`"'=HYPERLINK(""https://attacker.test"")"`);
  });
});

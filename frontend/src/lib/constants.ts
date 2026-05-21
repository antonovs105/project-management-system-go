import type { TicketPriority, TicketStatus, TicketType } from "../types";

export const ticketStatuses: Array<{ id: TicketStatus; label: string }> = [
  { id: "open", label: "Open" },
  { id: "in_progress", label: "In Progress" },
  { id: "review", label: "Review" },
  { id: "done", label: "Done" },
];

export const ticketPriorities: Array<{ id: TicketPriority; label: string }> = [
  { id: "low", label: "Low" },
  { id: "medium", label: "Medium" },
  { id: "high", label: "High" },
  { id: "urgent", label: "Urgent" },
];

export const ticketTypes: Array<{ id: TicketType; label: string }> = [
  { id: "epic", label: "Epic" },
  { id: "task", label: "Task" },
  { id: "subtask", label: "Subtask" },
];

export const projectRoles = [
  { id: "owner", label: "Owner" },
  { id: "manager", label: "Manager" },
  { id: "developer", label: "Developer" },
  { id: "viewer", label: "Viewer" },
] as const;

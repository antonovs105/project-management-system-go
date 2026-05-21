import type { TicketPriority, TicketStatus, TicketType } from "../types";
import { Badge } from "./ui";

function statusClass(status: TicketStatus): string {
  switch (status) {
    case "done":
      return "border-zinc-300 bg-zinc-950 text-white";
    case "review":
      return "border-zinc-300 bg-zinc-200 text-zinc-900";
    case "in_progress":
      return "border-zinc-300 bg-white text-zinc-900";
    default:
      return "border-zinc-200 bg-zinc-50 text-zinc-600";
  }
}

function priorityClass(priority: TicketPriority): string {
  switch (priority) {
    case "urgent":
      return "border-zinc-950 bg-zinc-950 text-white";
    case "high":
      return "border-zinc-300 bg-zinc-200 text-zinc-950";
    case "medium":
      return "border-zinc-200 bg-white text-zinc-700";
    default:
      return "border-zinc-200 bg-zinc-50 text-zinc-500";
  }
}

function typeClass(type: TicketType): string {
  switch (type) {
    case "epic":
      return "border-zinc-950 bg-zinc-950 text-white";
    case "subtask":
      return "border-zinc-200 bg-zinc-100 text-zinc-600";
    default:
      return "border-zinc-200 bg-white text-zinc-900";
  }
}

function label(value: string): string {
  return value.replace(/_/g, " ");
}

export function StatusBadge({
  value,
  kind,
}: {
  value: TicketStatus | TicketPriority | TicketType;
  kind: "status" | "priority" | "type";
}) {
  const tone = kind === "status" ? statusClass(value as TicketStatus) : kind === "priority" ? priorityClass(value as TicketPriority) : typeClass(value as TicketType);
  return <Badge className={tone}>{label(value)}</Badge>;
}

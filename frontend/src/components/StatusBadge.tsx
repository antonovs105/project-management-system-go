import type { TicketPriority, TicketStatus, TicketType } from "../types";
import { Badge } from "./ui";

function statusClass(status: TicketStatus): string {
  switch (status) {
    case "done":
      return "bg-emerald-100 text-emerald-800";
    case "review":
      return "bg-indigo-100 text-indigo-800";
    case "in_progress":
      return "bg-cyan-100 text-cyan-800";
    default:
      return "bg-slate-100 text-slate-700";
  }
}

function priorityClass(priority: TicketPriority): string {
  switch (priority) {
    case "urgent":
      return "bg-red-100 text-red-800";
    case "high":
      return "bg-orange-100 text-orange-800";
    case "medium":
      return "bg-amber-100 text-amber-800";
    default:
      return "bg-slate-100 text-slate-700";
  }
}

function typeClass(type: TicketType): string {
  switch (type) {
    case "epic":
      return "bg-violet-100 text-violet-800";
    case "subtask":
      return "bg-slate-200 text-slate-700";
    default:
      return "bg-blue-100 text-blue-800";
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

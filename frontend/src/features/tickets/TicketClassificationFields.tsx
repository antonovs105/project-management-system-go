import { SelectField } from "../../components/ui";
import { ticketPriorities, ticketStatuses, ticketTypes } from "../../lib/constants";
import type { TicketPriority, TicketStatus, TicketType } from "../../types";

type Labels = { status: string; priority: string; type: string };

export function TicketClassificationFields({
  status,
  priority,
  type,
  onStatusChange,
  onPriorityChange,
  onTypeChange,
  labels = { status: "Status", priority: "Priority", type: "Type" },
  disabled = false,
}: {
  status?: TicketStatus;
  priority: TicketPriority;
  type: TicketType;
  onStatusChange?: (value: TicketStatus) => void;
  onPriorityChange: (value: TicketPriority) => void;
  onTypeChange: (value: TicketType) => void;
  labels?: Labels;
  disabled?: boolean;
}) {
  const columns = status === undefined ? "sm:grid-cols-2" : "md:grid-cols-3";
  return (
    <div className={`grid gap-4 ${columns}`}>
      {status !== undefined && onStatusChange ? (
        <SelectField label={labels.status} value={status} onChange={(event) => onStatusChange(event.target.value as TicketStatus)} disabled={disabled}>
          {ticketStatuses.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
        </SelectField>
      ) : null}
      <SelectField label={labels.type} value={type} onChange={(event) => onTypeChange(event.target.value as TicketType)} disabled={disabled}>
        {ticketTypes.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
      </SelectField>
      <SelectField label={labels.priority} value={priority} onChange={(event) => onPriorityChange(event.target.value as TicketPriority)} disabled={disabled}>
        {ticketPriorities.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
      </SelectField>
    </div>
  );
}

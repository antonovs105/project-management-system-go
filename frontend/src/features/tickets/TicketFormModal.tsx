import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { api, errorMessage } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { ticketPriorities, ticketTypes } from "../../lib/constants";
import { fieldLimits } from "../../lib/limits";
import type { ID, Ticket, TicketPriority, TicketType } from "../../types";
import { Button, ErrorState, Modal, SelectField, TextAreaField, TextField } from "../../components/ui";

function parentCandidates(type: TicketType, tickets: Ticket[]): Ticket[] {
  if (type === "task") {
    return tickets.filter((ticket) => ticket.type === "epic");
  }
  if (type === "subtask") {
    return tickets.filter((ticket) => ticket.type === "task");
  }
  return [];
}

export function TicketFormModal({
  projectId,
  tickets,
  open,
  onClose,
}: {
  projectId: ID;
  tickets: Ticket[];
  open: boolean;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<TicketType>("task");
  const [priority, setPriority] = useState<TicketPriority>("medium");
  const [parentId, setParentId] = useState("");
  const [assigneeId, setAssigneeId] = useState("");

  const parents = useMemo(() => parentCandidates(type, tickets), [tickets, type]);

  const createTicket = useMutation({
    mutationFn: (payload: {
      title: string;
      description: string;
      type: TicketType;
      priority: TicketPriority;
      parent_id: ID | null;
      assignee_id: ID | null;
    }) => api.createTicket(projectId, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) });
      await queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) });
      setTitle("");
      setDescription("");
      setType("task");
      setPriority("medium");
      setParentId("");
      setAssigneeId("");
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    createTicket.mutate({
      title: title.trim(),
      description: description.trim(),
      type,
      priority,
      parent_id: parentId || null,
      assignee_id: assigneeId.trim() || null,
    });
  }

  return (
    <Modal
      open={open}
      title="Create Ticket"
      onClose={onClose}
      formId="create-ticket"
      onSubmit={submit}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" form="create-ticket" tone="primary" disabled={createTicket.isPending || !title.trim()}>
            Create
          </Button>
        </>
      }
    >
      <div className="grid gap-4">
        {createTicket.isError ? (
          <ErrorState title="Could not create ticket" body={errorMessage(createTicket.error, "Ticket creation failed.")} />
        ) : null}
        <TextField
          label="Title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          maxLength={fieldLimits.ticketTitleMaxLength}
          required
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField label="Type" value={type} onChange={(event) => setType(event.target.value as TicketType)}>
            {ticketTypes.map((item) => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </SelectField>
          <SelectField
            label="Priority"
            value={priority}
            onChange={(event) => setPriority(event.target.value as TicketPriority)}
          >
            {ticketPriorities.map((item) => (
              <option key={item.id} value={item.id}>
                {item.label}
              </option>
            ))}
          </SelectField>
        </div>
        {type !== "epic" ? (
          <SelectField label="Parent" value={parentId} onChange={(event) => setParentId(event.target.value)}>
            <option value="">None</option>
            {parents.map((ticket) => (
              <option key={ticket.id} value={ticket.id}>
                {ticket.title}
              </option>
            ))}
          </SelectField>
        ) : null}
        <TextField label="Assignee ID" value={assigneeId} onChange={(event) => setAssigneeId(event.target.value)} />
        <TextAreaField
          label="Description"
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          maxLength={fieldLimits.ticketDescriptionMaxLength}
          placeholder="What needs to happen?"
        />
      </div>
    </Modal>
  );
}

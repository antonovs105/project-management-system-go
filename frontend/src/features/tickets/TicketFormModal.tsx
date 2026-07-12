import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { api, errorMessage } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { fieldLimits } from "../../lib/limits";
import { useI18n } from "../../lib/i18n-context";
import type { ID, Label, ProjectMember, Ticket, TicketPriority, TicketType } from "../../types";
import { Button, ErrorState, Modal, SelectField, TextAreaField, TextField } from "../../components/ui";
import { MemberAssigneeSelect } from "./MemberAssigneeSelect";
import { TicketClassificationFields } from "./TicketClassificationFields";

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
  members,
  labels,
  open,
  onClose,
}: {
  projectId: ID;
  tickets: Ticket[];
  members: ProjectMember[];
  labels: Label[];
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<TicketType>("task");
  const [priority, setPriority] = useState<TicketPriority>("medium");
  const [parentId, setParentId] = useState("");
  const [assigneeId, setAssigneeId] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [labelIds, setLabelIds] = useState<ID[]>([]);

  const parents = useMemo(() => parentCandidates(type, tickets), [tickets, type]);

  const createTicket = useMutation({
    mutationFn: (payload: {
      title: string;
      description: string;
      type: TicketType;
      priority: TicketPriority;
      parent_id: ID | null;
      assignee_id: ID | null;
      due_date: string;
      label_ids: ID[];
    }) => api.createTicket(projectId, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) });
      await queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) });
      setTitle("");
      setDescription("");
      setType("task");
      setPriority("medium");
      setParentId("");
      setAssigneeId("");
      setDueDate("");
      setLabelIds([]);
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
      due_date: dueDate,
      label_ids: labelIds,
    });
  }

  return (
    <Modal
      open={open}
      title={t("ticket.createTitle")}
      onClose={onClose}
      formId="create-ticket"
      onSubmit={submit}
      footer={
        <>
          <Button onClick={onClose}>{t("actions.cancel")}</Button>
          <Button type="submit" form="create-ticket" tone="primary" disabled={createTicket.isPending || !title.trim()}>
            {t("actions.create")}
          </Button>
        </>
      }
    >
      <div className="grid gap-4">
        {createTicket.isError ? (
          <ErrorState title={t("ticket.createFailed")} body={errorMessage(createTicket.error, t("ticket.creationFailed"))} />
        ) : null}
        <TextField
          label={t("ticket.title")}
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          maxLength={fieldLimits.ticketTitleMaxLength}
          required
        />
        <TicketClassificationFields
          type={type}
          priority={priority}
          onTypeChange={setType}
          onPriorityChange={setPriority}
          labels={{ status: "", type: t("ticket.type"), priority: t("ticket.priority") }}
        />
        {type !== "epic" ? (
          <SelectField label={t("ticket.parent")} value={parentId} onChange={(event) => setParentId(event.target.value)}>
            <option value="">{t("ticket.none")}</option>
            {parents.map((ticket) => (
              <option key={ticket.id} value={ticket.id}>
                {ticket.title}
              </option>
            ))}
          </SelectField>
        ) : null}
        <MemberAssigneeSelect members={members} value={assigneeId} onChange={setAssigneeId} />
        <TextField label={t("ticket.dueDate")} type="date" value={dueDate} onChange={(event) => setDueDate(event.target.value)} />
        {labels.length > 0 ? (
          <fieldset className="grid gap-2">
            <legend className="text-sm font-medium text-zinc-700">{t("ticket.labels")}</legend>
            <div className="flex flex-wrap gap-2">
              {labels.map((label) => (
                <label key={label.id} className="flex cursor-pointer items-center gap-2 rounded-full border border-zinc-200 px-3 py-1.5 text-sm">
                  <input
                    type="checkbox"
                    checked={labelIds.includes(label.id)}
                    onChange={(event) => setLabelIds((current) => event.target.checked ? [...current, label.id] : current.filter((id) => id !== label.id))}
                  />
                  <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: label.color }} />
                  {label.name}
                </label>
              ))}
            </div>
          </fieldset>
        ) : null}
        <TextAreaField
          label={t("ticket.description")}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          maxLength={fieldLimits.ticketDescriptionMaxLength}
          placeholder={t("ticket.descriptionPlaceholder")}
        />
      </div>
    </Modal>
  );
}

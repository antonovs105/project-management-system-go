import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquare, Save, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { api, errorMessage, type UpdateTicketPayload } from "../../lib/api";
import { ticketPriorities, ticketStatuses, ticketTypes } from "../../lib/constants";
import { compactId, relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type { ID, Ticket, TicketPriority, TicketStatus, TicketType } from "../../types";
import { StatusBadge } from "../../components/StatusBadge";
import { Button, ErrorState, IconButton, LoadingState, SelectField, TextAreaField, TextField } from "../../components/ui";

function parentCandidates(type: TicketType, ticketId: ID, tickets: Ticket[]): Ticket[] {
  if (type === "task") {
    return tickets.filter((ticket) => ticket.type === "epic" && ticket.id !== ticketId);
  }
  if (type === "subtask") {
    return tickets.filter((ticket) => ticket.type === "task" && ticket.id !== ticketId);
  }
  return [];
}

function TicketEditor({
  projectId,
  ticket,
  tickets,
  onClose,
}: {
  projectId: ID;
  ticket: Ticket;
  tickets: Ticket[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState(ticket.title);
  const [description, setDescription] = useState(ticket.description);
  const [status, setStatus] = useState<TicketStatus>(ticket.status);
  const [priority, setPriority] = useState<TicketPriority>(ticket.priority);
  const [type, setType] = useState<TicketType>(ticket.type);
  const [parentId, setParentId] = useState(ticket.parent_id || "");

  const parents = useMemo(() => parentCandidates(type, ticket.id, tickets), [ticket.id, tickets, type]);

  const updateTicket = useMutation({
    mutationFn: (payload: UpdateTicketPayload) => api.updateTicket(ticket.id, payload),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ticket(ticket.id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      toast.success("Ticket updated");
    },
  });

  const deleteTicket = useMutation({
    mutationFn: () => api.deleteTicket(ticket.id),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      toast.success("Ticket deleted");
      onClose();
    },
  });

  function submitTicket(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    updateTicket.mutate({
      title: title.trim(),
      description: description.trim(),
      status,
      priority,
      type,
      parent_id: type === "epic" ? null : parentId || null,
    });
  }

  return (
    <form className="grid gap-4" onSubmit={submitTicket}>
      {updateTicket.isError ? (
        <ErrorState title="Could not update ticket" body={errorMessage(updateTicket.error, "Ticket update failed.")} />
      ) : null}
      {deleteTicket.isError ? (
        <ErrorState title="Could not delete ticket" body={errorMessage(deleteTicket.error, "Ticket delete failed.")} />
      ) : null}
      <TextField label="Title" value={title} onChange={(event) => setTitle(event.target.value)} required />
      <div className="grid gap-4 sm:grid-cols-3">
        <SelectField label="Status" value={status} onChange={(event) => setStatus(event.target.value as TicketStatus)}>
          {ticketStatuses.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label="Priority" value={priority} onChange={(event) => setPriority(event.target.value as TicketPriority)}>
          {ticketPriorities.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
        <SelectField label="Type" value={type} onChange={(event) => setType(event.target.value as TicketType)}>
          {ticketTypes.map((item) => (
            <option key={item.id} value={item.id}>
              {item.label}
            </option>
          ))}
        </SelectField>
      </div>
      {type !== "epic" ? (
        <SelectField label="Parent" value={parentId} onChange={(event) => setParentId(event.target.value)}>
          <option value="">None</option>
          {parents.map((candidate) => (
            <option key={candidate.id} value={candidate.id}>
              {candidate.title}
            </option>
          ))}
        </SelectField>
      ) : null}
      <TextAreaField label="Description" value={description} onChange={(event) => setDescription(event.target.value)} />
      <div className="flex justify-between gap-2">
        <Button
          tone="danger"
          onClick={() => {
            if (window.confirm("Delete this ticket?")) {
              deleteTicket.mutate();
            }
          }}
          disabled={deleteTicket.isPending}
        >
          <Trash2 size={16} />
          Delete
        </Button>
        <Button type="submit" tone="primary" disabled={updateTicket.isPending || !title.trim()}>
          <Save size={16} />
          Save
        </Button>
      </div>
    </form>
  );
}

export function TicketDetailPanel({
  projectId,
  ticketId,
  tickets,
  onClose,
}: {
  projectId: ID;
  ticketId: ID;
  tickets: Ticket[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [comment, setComment] = useState("");

  const ticket = useQuery({
    queryKey: queryKeys.ticket(ticketId),
    queryFn: () => api.getTicket(ticketId),
  });

  const comments = useQuery({
    queryKey: queryKeys.comments(ticketId),
    queryFn: () => api.listComments(ticketId),
  });

  const createComment = useMutation({
    mutationFn: (content: string) => api.createComment(ticketId, content),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.comments(ticketId) });
      setComment("");
    },
  });

  const removeComment = useMutation({
    mutationFn: api.deleteComment,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.comments(ticketId) });
    },
  });

  function submitComment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = comment.trim();
    if (trimmed) {
      createComment.mutate(trimmed);
    }
  }

  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-slate-950/30">
      <button type="button" aria-label="Close ticket panel" className="hidden flex-1 md:block" onClick={onClose} />
      <aside className="flex h-full w-full max-w-2xl flex-col overflow-hidden bg-white shadow-xl">
        <header className="flex items-start justify-between border-b border-slate-200 px-5 py-4">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap gap-1.5">
              {ticket.data ? (
                <>
                  <StatusBadge value={ticket.data.type} kind="type" />
                  <StatusBadge value={ticket.data.status} kind="status" />
                  <StatusBadge value={ticket.data.priority} kind="priority" />
                </>
              ) : null}
            </div>
            <h2 className="truncate text-lg font-semibold text-slate-950">
              {ticket.data?.title || `Ticket ${compactId(ticketId)}`}
            </h2>
          </div>
          <IconButton label="Close ticket" onClick={onClose}>
            <X size={18} />
          </IconButton>
        </header>

        <div className="flex-1 overflow-y-auto px-5 py-5">
          {ticket.isLoading ? <LoadingState label="Loading ticket" /> : null}
          {ticket.isError ? (
            <ErrorState title="Could not load ticket" body={errorMessage(ticket.error, "Ticket request failed.")} />
          ) : null}

          {ticket.data ? (
            <div className="grid gap-6">
              <TicketEditor
                key={`${ticket.data.id}:${ticket.data.updated_at}`}
                projectId={projectId}
                ticket={ticket.data}
                tickets={tickets}
                onClose={onClose}
              />

              <section className="border-t border-slate-200 pt-5">
                <div className="mb-3 flex items-center gap-2">
                  <MessageSquare size={18} className="text-slate-500" />
                  <h3 className="font-semibold text-slate-950">Comments</h3>
                </div>
                <form className="mb-4 grid gap-2" onSubmit={submitComment}>
                  <TextAreaField
                    label="Add comment"
                    value={comment}
                    onChange={(event) => setComment(event.target.value)}
                    placeholder="Write an update"
                  />
                  <div className="flex justify-end">
                    <Button type="submit" tone="primary" disabled={createComment.isPending || !comment.trim()}>
                      Comment
                    </Button>
                  </div>
                </form>

                {comments.isLoading ? <LoadingState label="Loading comments" /> : null}
                {comments.isError ? (
                  <ErrorState title="Could not load comments" body={errorMessage(comments.error, "Comment request failed.")} />
                ) : null}
                <div className="space-y-2">
                  {comments.data?.map((item) => (
                    <div key={item.id} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
                      <div className="mb-1 flex items-center justify-between gap-3 text-xs text-slate-500">
                        <span>{compactId(item.author_id)}</span>
                        <div className="flex items-center gap-2">
                          <span>{relativeDate(item.created_at)}</span>
                          <IconButton
                            label="Delete comment"
                            className="h-7 w-7"
                            onClick={() => removeComment.mutate(item.id)}
                            disabled={removeComment.isPending}
                          >
                            <Trash2 size={14} />
                          </IconButton>
                        </div>
                      </div>
                      <p className="whitespace-pre-wrap text-sm text-slate-800">{item.content}</p>
                    </div>
                  ))}
                  {comments.data?.length === 0 ? <p className="text-sm text-slate-500">No comments yet.</p> : null}
                </div>
              </section>
            </div>
          ) : null}
        </div>
      </aside>
    </div>
  );
}

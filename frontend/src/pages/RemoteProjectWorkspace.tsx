import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink, Plus, RefreshCw, Save, Trash2, X } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";
import { StatusBadge } from "../components/StatusBadge";
import { Badge, Button, EmptyState, ErrorState, IconButton, LoadingState, Modal, Panel, SelectField, TextAreaField, TextField } from "../components/ui";
import { TicketBoard } from "../features/tickets/TicketBoard";
import { api, errorMessage } from "../lib/api";
import { ticketPriorities, ticketStatuses, ticketTypes } from "../lib/constants";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import type { Ticket, TicketPriority, TicketStatus, TicketType } from "../types";

function updateTicketCache(tickets: Ticket[] | undefined, ticket: Ticket): Ticket[] {
  const current = tickets || [];
  const exists = current.some((item) => item.id === ticket.id);
  if (!exists) {
    return [ticket, ...current];
  }
  return current.map((item) => (item.id === ticket.id ? ticket : item));
}

export function RemoteProjectWorkspace() {
  const { projectId = "" } = useParams();
  const queryClient = useQueryClient();
  const { t } = useI18n();
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(null);
  const [draft, setDraft] = useState({
    title: "",
    description: "",
    status: "open" as TicketStatus,
    priority: "medium" as TicketPriority,
    type: "task" as TicketType,
  });
  const [createDraft, setCreateDraft] = useState({
    title: "",
    description: "",
    priority: "medium" as TicketPriority,
    type: "task" as TicketType,
  });

  const project = useQuery({
    queryKey: queryKeys.remoteProject(projectId),
    queryFn: () => api.getRemoteProject(projectId),
    enabled: Boolean(projectId),
  });

  const tickets = useQuery({
    queryKey: queryKeys.remoteTickets(projectId),
    queryFn: () => api.listRemoteProjectTickets(projectId),
    enabled: Boolean(projectId),
  });

  const selectedTicket = useMemo(
    () => (tickets.data || []).find((ticket) => ticket.id === selectedTicketId) || null,
    [selectedTicketId, tickets.data],
  );

  function cacheTicket(ticket: Ticket) {
    queryClient.setQueryData<Ticket[]>(queryKeys.remoteTickets(projectId), (current) => updateTicketCache(current, ticket));
  }

  const createTicket = useMutation({
    mutationFn: () =>
      api.createRemoteTicket(projectId, {
        title: createDraft.title.trim(),
        description: createDraft.description.trim(),
        priority: createDraft.priority,
        type: createDraft.type,
      }),
    onSuccess: async (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      setCreateOpen(false);
      setCreateDraft({ title: "", description: "", priority: "medium", type: "task" });
      toast.success(t("remoteWorkspace.ticketQueued"));
      await queryClient.invalidateQueries({ queryKey: queryKeys.remoteTickets(projectId) });
    },
  });

  const updateTicket = useMutation({
    mutationFn: () =>
      selectedTicket
        ? api.updateRemoteTicket(projectId, selectedTicket.id, {
            title: draft.title.trim(),
            description: draft.description.trim(),
            status: draft.status,
            priority: draft.priority,
            type: draft.type,
            is_resolved: draft.status === "done",
          })
        : Promise.reject(new Error("No ticket selected")),
    onSuccess: async (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      toast.success(t("remoteWorkspace.ticketQueued"));
      await queryClient.invalidateQueries({ queryKey: queryKeys.remoteTickets(projectId) });
    },
  });

  const moveTicket = useMutation({
    mutationFn: ({ ticketId, status }: { ticketId: string; status: TicketStatus }) => api.moveRemoteTicket(projectId, ticketId, { status }),
    onMutate: ({ ticketId, status }) => {
      queryClient.setQueryData<Ticket[]>(queryKeys.remoteTickets(projectId), (current) =>
        (current || []).map((ticket) => (ticket.id === ticketId ? { ...ticket, status, is_resolved: status === "done" } : ticket)),
      );
    },
    onSuccess: async (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      await queryClient.invalidateQueries({ queryKey: queryKeys.remoteTickets(projectId) });
    },
  });

  const deleteTicket = useMutation({
    mutationFn: () => (selectedTicket ? api.deleteRemoteTicket(projectId, selectedTicket.id) : Promise.reject(new Error("No ticket selected"))),
    onSuccess: async () => {
      if (selectedTicket) {
        queryClient.setQueryData<Ticket[]>(queryKeys.remoteTickets(projectId), (current) =>
          (current || []).filter((ticket) => ticket.id !== selectedTicket.id),
        );
      }
      setSelectedTicketId(null);
      toast.success(t("remoteWorkspace.ticketQueued"));
      await queryClient.invalidateQueries({ queryKey: queryKeys.remoteTickets(projectId) });
    },
  });

  function openTicket(ticketId: string) {
    const ticket = (tickets.data || []).find((item) => item.id === ticketId);
    if (!ticket) {
      return;
    }
    setSelectedTicketId(ticketId);
    setDraft({
      title: ticket.title,
      description: ticket.description,
      status: ticket.status,
      priority: ticket.priority,
      type: ticket.type,
    });
  }

  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (createDraft.title.trim()) {
      createTicket.mutate();
    }
  }

  function submitUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (selectedTicket && draft.title.trim()) {
      updateTicket.mutate();
    }
  }

  const loading = project.isLoading || tickets.isLoading;
  const remoteProject = project.data;
  const remoteTickets = tickets.data || [];

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 rounded-3xl border border-zinc-200 bg-white p-5 shadow-sm lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <Link to="/projects" className="mb-3 inline-flex items-center gap-2 text-sm font-medium text-zinc-500 hover:text-zinc-950">
            <ArrowLeft size={16} />
            {t("remoteWorkspace.back")}
          </Link>
          <div className="mb-2 flex flex-wrap gap-2">
            <Badge className="border-zinc-950 bg-zinc-950 text-white">{t("projects.remoteBadge")}</Badge>
            {remoteProject?.role ? <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{remoteProject.role}</Badge> : null}
          </div>
          <h1 className="truncate text-3xl font-semibold tracking-tight text-zinc-950">
            {remoteProject?.project_name || t("remoteWorkspace.title")}
          </h1>
          <p className="mt-1 truncate text-sm text-zinc-500">{remoteProject?.project_ap_id || t("remoteWorkspace.subtitle")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {remoteProject?.project_ap_id ? (
            <a
              href={remoteProject.project_ap_id}
              target="_blank"
              rel="noreferrer"
              className="focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-full border border-zinc-200 bg-white px-4 text-sm font-medium text-zinc-800 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
            >
              <ExternalLink size={16} />
              {t("remoteWorkspace.activityPub")}
            </a>
          ) : null}
          <Button onClick={() => tickets.refetch()} disabled={tickets.isFetching}>
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
          <Button tone="primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} />
            {t("remoteWorkspace.createTicket")}
          </Button>
        </div>
      </div>

      {loading ? <LoadingState label={t("remoteWorkspace.loading")} /> : null}

      {project.isError ? (
        <ErrorState title={t("remoteWorkspace.loadFailed")} body={errorMessage(project.error, t("remoteWorkspace.loadFailedBody"))} />
      ) : null}

      {tickets.isError ? (
        <ErrorState title={t("remoteWorkspace.ticketsFailed")} body={errorMessage(tickets.error, t("remoteWorkspace.ticketsFailedBody"))} />
      ) : null}

      {!loading && !project.isError && !tickets.isError ? (
        remoteTickets.length > 0 ? (
          <TicketBoard
            tickets={remoteTickets}
            members={[]}
            onOpenTicket={openTicket}
            onMoveTicket={(ticketId, status) => moveTicket.mutate({ ticketId, status })}
            emptyAction={null}
          />
        ) : (
          <EmptyState
            title={t("remoteWorkspace.emptyTitle")}
            body={t("remoteWorkspace.emptyBody")}
            action={
              <Button tone="primary" onClick={() => setCreateOpen(true)}>
                <Plus size={16} />
                {t("remoteWorkspace.createTicket")}
              </Button>
            }
          />
        )
      ) : null}

      {selectedTicket ? (
        <div className="fixed inset-y-0 right-0 z-40 flex w-full max-w-2xl flex-col border-l border-zinc-200 bg-white shadow-2xl">
          <div className="flex items-start justify-between border-b border-zinc-200 px-6 py-5">
            <div className="min-w-0">
              <div className="mb-2 flex flex-wrap gap-1.5">
                <StatusBadge value={draft.type} kind="type" />
                <StatusBadge value={draft.status} kind="status" />
                <StatusBadge value={draft.priority} kind="priority" />
              </div>
              <h2 className="truncate text-xl font-semibold text-zinc-950">{selectedTicket.title}</h2>
            </div>
            <IconButton label={t("nav.close")} onClick={() => setSelectedTicketId(null)}>
              <X size={18} />
            </IconButton>
          </div>
          <form id="remote-ticket-edit" onSubmit={submitUpdate} className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
            <div className="grid gap-4">
              {updateTicket.isError ? (
                <ErrorState
                  title={t("remoteWorkspace.updateFailed")}
                  body={errorMessage(updateTicket.error, t("remoteWorkspace.updateFailedBody"))}
                />
              ) : null}
              {deleteTicket.isError ? (
                <ErrorState
                  title={t("remoteWorkspace.deleteFailed")}
                  body={errorMessage(deleteTicket.error, t("remoteWorkspace.deleteFailedBody"))}
                />
              ) : null}
              <TextField
                label={t("remoteWorkspace.ticketTitle")}
                value={draft.title}
                onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))}
                maxLength={fieldLimits.ticketTitleMaxLength}
                required
              />
              <div className="grid gap-4 md:grid-cols-3">
                <SelectField
                  label={t("remoteWorkspace.status")}
                  value={draft.status}
                  onChange={(event) => setDraft((current) => ({ ...current, status: event.target.value as TicketStatus }))}
                >
                  {ticketStatuses.map((status) => (
                    <option key={status.id} value={status.id}>
                      {status.label}
                    </option>
                  ))}
                </SelectField>
                <SelectField
                  label={t("remoteWorkspace.priority")}
                  value={draft.priority}
                  onChange={(event) => setDraft((current) => ({ ...current, priority: event.target.value as TicketPriority }))}
                >
                  {ticketPriorities.map((priority) => (
                    <option key={priority.id} value={priority.id}>
                      {priority.label}
                    </option>
                  ))}
                </SelectField>
                <SelectField
                  label={t("remoteWorkspace.type")}
                  value={draft.type}
                  onChange={(event) => setDraft((current) => ({ ...current, type: event.target.value as TicketType }))}
                >
                  {ticketTypes.map((type) => (
                    <option key={type.id} value={type.id}>
                      {type.label}
                    </option>
                  ))}
                </SelectField>
              </div>
              <TextAreaField
                label={t("remoteWorkspace.description")}
                value={draft.description}
                onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))}
                maxLength={fieldLimits.ticketDescriptionMaxLength}
              />
              <Panel className="p-4">
                <div className="text-xs font-medium uppercase tracking-wide text-zinc-400">ActivityPub</div>
                <p className="mt-2 break-all text-sm text-zinc-600">{selectedTicket.ap_id}</p>
              </Panel>
            </div>
          </form>
          <div className="flex items-center justify-between gap-3 border-t border-zinc-200 px-6 py-4">
            <Button tone="danger" onClick={() => deleteTicket.mutate()} disabled={deleteTicket.isPending}>
              <Trash2 size={16} />
              {t("actions.delete")}
            </Button>
            <Button type="submit" form="remote-ticket-edit" tone="primary" disabled={updateTicket.isPending || !draft.title.trim()}>
              <Save size={16} />
              {t("actions.save")}
            </Button>
          </div>
        </div>
      ) : null}

      <Modal
        open={createOpen}
        title={t("remoteWorkspace.createTicket")}
        onClose={() => setCreateOpen(false)}
        formId="remote-create-ticket"
        onSubmit={submitCreate}
        footer={
          <>
            <Button onClick={() => setCreateOpen(false)}>{t("actions.cancel")}</Button>
            <Button type="submit" form="remote-create-ticket" tone="primary" disabled={createTicket.isPending || !createDraft.title.trim()}>
              {t("actions.create")}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          {createTicket.isError ? (
            <ErrorState title={t("remoteWorkspace.createFailed")} body={errorMessage(createTicket.error, t("remoteWorkspace.createFailedBody"))} />
          ) : null}
          <TextField
            label={t("remoteWorkspace.ticketTitle")}
            value={createDraft.title}
            onChange={(event) => setCreateDraft((current) => ({ ...current, title: event.target.value }))}
            maxLength={fieldLimits.ticketTitleMaxLength}
            required
          />
          <div className="grid gap-4 md:grid-cols-2">
            <SelectField
              label={t("remoteWorkspace.priority")}
              value={createDraft.priority}
              onChange={(event) => setCreateDraft((current) => ({ ...current, priority: event.target.value as TicketPriority }))}
            >
              {ticketPriorities.map((priority) => (
                <option key={priority.id} value={priority.id}>
                  {priority.label}
                </option>
              ))}
            </SelectField>
            <SelectField
              label={t("remoteWorkspace.type")}
              value={createDraft.type}
              onChange={(event) => setCreateDraft((current) => ({ ...current, type: event.target.value as TicketType }))}
            >
              {ticketTypes.map((type) => (
                <option key={type.id} value={type.id}>
                  {type.label}
                </option>
              ))}
            </SelectField>
          </div>
          <TextAreaField
            label={t("remoteWorkspace.description")}
            value={createDraft.description}
            onChange={(event) => setCreateDraft((current) => ({ ...current, description: event.target.value }))}
            maxLength={fieldLimits.ticketDescriptionMaxLength}
          />
        </div>
      </Modal>
    </div>
  );
}

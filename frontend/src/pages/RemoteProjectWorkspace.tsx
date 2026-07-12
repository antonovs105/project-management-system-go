import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ExternalLink, LockKeyhole, Plus, RefreshCw, Save, Trash2, X } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";
import { StatusBadge } from "../components/StatusBadge";
import { Badge, Button, ErrorState, IconButton, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";
import { TicketBoard } from "../features/tickets/TicketBoard";
import { TicketClassificationFields } from "../features/tickets/TicketClassificationFields";
import { ProjectTicketSummary } from "../features/projects/ProjectTicketSummary";
import { api, errorMessage } from "../lib/api";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import type { ID, RemoteProject, RemoteTicket, TicketPriority, TicketStatus, TicketType } from "../types";

const ticketCreatePermission = "tickets.create";
const ticketUpdatePermission = "tickets.update";
const ticketDeletePermission = "tickets.delete";
const remoteTicketPollMs = 5000;

const builtInRolePermissions: Record<string, string[]> = {
  owner: [
    "project.read",
    "project.update",
    "project.delete",
    "members.invite",
    "members.remove",
    "roles.manage",
    ticketCreatePermission,
    ticketUpdatePermission,
    ticketDeletePermission,
    "comments.create",
    "comments.moderate",
    "federation.delivery.retry",
  ],
  manager: [
    "project.read",
    "project.update",
    "members.invite",
    "members.remove",
    ticketCreatePermission,
    ticketUpdatePermission,
    ticketDeletePermission,
    "comments.create",
    "comments.moderate",
    "federation.delivery.retry",
  ],
  developer: ["project.read", ticketCreatePermission, ticketUpdatePermission, "comments.create"],
  viewer: ["project.read"],
};

function updateTicketCache(tickets: RemoteTicket[] | undefined, ticket: RemoteTicket): RemoteTicket[] {
  const current = tickets || [];
  const exists = current.some((item) => item.id === ticket.id);
  if (!exists) {
    return [ticket, ...current];
  }
  return current.map((item) => (item.id === ticket.id ? ticket : item));
}

function moveTicketsOptimistically(tickets: RemoteTicket[], ticketId: ID, status: TicketStatus): RemoteTicket[] {
  return tickets.map((ticket) => (ticket.id === ticketId ? { ...ticket, status, is_resolved: status === "done" } : ticket));
}

function remoteProjectTitle(project?: RemoteProject): string {
  if (!project) {
    return "Remote project";
  }
  const name = project.project_name.trim();
  if (name && name !== project.project_ap_id && !/^https?:\/\//i.test(name)) {
    return name;
  }
  if (project.remote_handle) {
    return project.remote_handle;
  }
  try {
    const url = new URL(project.project_ap_id);
    return url.pathname.split("/").filter(Boolean).pop() || url.host;
  } catch {
    return project.project_ap_id;
  }
}

function projectPermissions(project?: RemoteProject): Set<string> {
  const explicit = project?.role_permissions || [];
  if (explicit.length > 0) {
    return new Set(explicit);
  }
  return new Set(builtInRolePermissions[(project?.role || "").toLowerCase()] || ["project.read"]);
}

export function RemoteProjectWorkspace() {
  const { projectId = "" } = useParams();
  const queryClient = useQueryClient();
  const { t, relativeDate } = useI18n();
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
    refetchInterval: remoteTicketPollMs,
    refetchIntervalInBackground: false,
    refetchOnReconnect: true,
    refetchOnWindowFocus: true,
  });

  const permissions = useMemo(() => projectPermissions(project.data), [project.data]);
  const canCreateTickets = permissions.has(ticketCreatePermission);
  const canUpdateTickets = permissions.has(ticketUpdatePermission);
  const canDeleteTickets = permissions.has(ticketDeletePermission);
  const remoteTickets = useMemo(() => tickets.data || [], [tickets.data]);
  const selectedTicket = useMemo(
    () => remoteTickets.find((ticket) => ticket.id === selectedTicketId) || null,
    [remoteTickets, selectedTicketId],
  );

  function cacheTicket(ticket: RemoteTicket) {
    queryClient.setQueryData<RemoteTicket[]>(queryKeys.remoteTickets(projectId), (current) => updateTicketCache(current, ticket));
  }

  function refreshRemoteTicketsSoon() {
    window.setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.remoteTickets(projectId) });
    }, 2500);
  }

  const createTicket = useMutation({
    mutationFn: () =>
      api.createRemoteTicket(projectId, {
        title: createDraft.title.trim(),
        description: createDraft.description.trim(),
        priority: createDraft.priority,
        type: createDraft.type,
      }),
    onSuccess: (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      setCreateOpen(false);
      setCreateDraft({ title: "", description: "", priority: "medium", type: "task" });
      toast.success(t("remoteWorkspace.ticketQueued"));
      refreshRemoteTicketsSoon();
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
    onSuccess: (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      toast.success(t("remoteWorkspace.ticketQueued"));
      refreshRemoteTicketsSoon();
    },
  });

  const moveTicket = useMutation({
    mutationFn: ({ ticketId, status }: { ticketId: string; status: TicketStatus }) => api.moveRemoteTicket(projectId, ticketId, { status }),
    onMutate: ({ ticketId, status }) => {
      const previous = queryClient.getQueryData<RemoteTicket[]>(queryKeys.remoteTickets(projectId));
      queryClient.setQueryData<RemoteTicket[]>(queryKeys.remoteTickets(projectId), (current = []) =>
        moveTicketsOptimistically(current, ticketId, status),
      );
      return { previous };
    },
    onError: (error, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.remoteTickets(projectId), context.previous);
      }
      toast.error(errorMessage(error, t("remoteWorkspace.updateFailedBody")));
    },
    onSuccess: (result) => {
      if (result.ticket) {
        cacheTicket(result.ticket);
      }
      refreshRemoteTicketsSoon();
    },
  });

  const deleteTicket = useMutation({
    mutationFn: () => (selectedTicket ? api.deleteRemoteTicket(projectId, selectedTicket.id) : Promise.reject(new Error("No ticket selected"))),
    onSuccess: () => {
      if (selectedTicket) {
        queryClient.setQueryData<RemoteTicket[]>(queryKeys.remoteTickets(projectId), (current) =>
          (current || []).filter((ticket) => ticket.id !== selectedTicket.id),
        );
      }
      setSelectedTicketId(null);
      toast.success(t("remoteWorkspace.ticketQueued"));
      refreshRemoteTicketsSoon();
    },
  });

  function openTicket(ticketId: string) {
    const ticket = remoteTickets.find((item) => item.id === ticketId);
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
    if (!canCreateTickets) {
      toast.error(t("remoteWorkspace.readOnlyBody"));
      return;
    }
    if (createDraft.title.trim()) {
      createTicket.mutate();
    }
  }

  function submitUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canUpdateTickets) {
      toast.error(t("remoteWorkspace.readOnlyBody"));
      return;
    }
    if (selectedTicket && draft.title.trim()) {
      updateTicket.mutate();
    }
  }

  function handleMove(ticketId: ID, status: TicketStatus) {
    const ticket = remoteTickets.find((item) => item.id === ticketId);
    if (!ticket) {
      return;
    }
    if (!canUpdateTickets) {
      toast.error(t("remoteWorkspace.readOnlyBody"));
      return;
    }
    if (ticket.status === status) {
      toast.error(t("remoteWorkspace.reorderUnsupported"));
      return;
    }
    moveTicket.mutate({ ticketId, status });
  }

  const loading = project.isLoading || tickets.isLoading;
  const remoteProject = project.data;
  const title = remoteProjectTitle(remoteProject);
  const readOnly = Boolean(remoteProject) && !canCreateTickets && !canUpdateTickets && !canDeleteTickets;
  const createDisabled = !remoteProject || !canCreateTickets;

  return (
    <div className="space-y-5">
      {!projectId ? <ErrorState title={t("remoteWorkspace.missing")} body={t("remoteWorkspace.missingBody")} /> : null}

      <div className="flex flex-col gap-4 rounded-3xl border border-zinc-200 bg-white p-5 shadow-sm xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0">
          <Link to="/projects" className="mb-3 inline-flex items-center gap-2 text-sm font-medium text-zinc-500 hover:text-zinc-950">
            <ArrowLeft size={16} />
            {t("remoteWorkspace.back")}
          </Link>
          <div className="mb-2 flex flex-wrap gap-2">
            <Badge className="border-zinc-950 bg-zinc-950 text-white">{t("projects.remoteBadge")}</Badge>
            {remoteProject?.role ? <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{remoteProject.role}</Badge> : null}
            {readOnly ? (
              <Badge className="inline-flex items-center gap-1 border-zinc-200 bg-zinc-50 text-zinc-500">
                <LockKeyhole size={12} />
                {t("remoteWorkspace.readOnly")}
              </Badge>
            ) : null}
          </div>
          <h1 className="truncate text-2xl font-semibold tracking-tight text-zinc-950">{title}</h1>
          <p className="mt-1 break-all text-sm text-zinc-500">{remoteProject?.project_ap_id || t("remoteWorkspace.subtitle")}</p>
          <div className="mt-4 flex flex-wrap gap-2">
            <ProjectTicketSummary tickets={remoteTickets} />
          </div>
          {remoteProject?.updated_at ? <p className="mt-3 text-xs text-zinc-400">{t("remoteWorkspace.updated")} {relativeDate(remoteProject.updated_at)}</p> : null}
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
          <Button
            tone="primary"
            onClick={() => setCreateOpen(true)}
            disabled={createDisabled}
            title={remoteProject && !canCreateTickets ? t("remoteWorkspace.noCreatePermission") : undefined}
          >
            <Plus size={16} />
            {t("remoteWorkspace.createTicket")}
          </Button>
        </div>
      </div>

      {readOnly ? (
        <Panel className="flex items-start gap-3 p-4 text-sm text-zinc-600">
          <LockKeyhole size={18} className="mt-0.5 shrink-0 text-zinc-400" />
          <div>
            <div className="font-semibold text-zinc-950">{t("remoteWorkspace.readOnly")}</div>
            <p className="mt-1">{t("remoteWorkspace.readOnlyBody")}</p>
          </div>
        </Panel>
      ) : null}

      {loading ? <LoadingState label={t("remoteWorkspace.loading")} /> : null}

      {project.isError ? (
        <ErrorState title={t("remoteWorkspace.loadFailed")} body={errorMessage(project.error, t("remoteWorkspace.loadFailedBody"))} />
      ) : null}

      {tickets.isError ? (
        <ErrorState title={t("remoteWorkspace.ticketsFailed")} body={errorMessage(tickets.error, t("remoteWorkspace.ticketsFailedBody"))} />
      ) : null}

      {!loading && !project.isError && !tickets.isError && remoteTickets.length === 0 ? (
        <Panel className="flex flex-col gap-3 p-4 text-sm text-zinc-600 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="font-semibold text-zinc-950">{t("remoteWorkspace.emptyTitle")}</div>
            <p className="mt-1">{t("remoteWorkspace.emptyBody")}</p>
          </div>
          {canCreateTickets ? (
            <Button tone="primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              {t("remoteWorkspace.createTicket")}
            </Button>
          ) : null}
        </Panel>
      ) : null}

      {!loading && !project.isError && !tickets.isError ? (
        <TicketBoard
          tickets={remoteTickets}
          members={[]}
          onOpenTicket={openTicket}
          onMoveTicket={handleMove}
          readOnly={!remoteProject || !canUpdateTickets}
          showColumnsWhenEmpty
          emptyAction={
            canCreateTickets ? (
              <Button tone="primary" onClick={() => setCreateOpen(true)}>
                <Plus size={16} />
                {t("remoteWorkspace.createTicket")}
              </Button>
            ) : null
          }
        />
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
                disabled={!canUpdateTickets}
                required
              />
              <TicketClassificationFields
                status={draft.status}
                priority={draft.priority}
                type={draft.type}
                onStatusChange={(status) => setDraft((current) => ({ ...current, status }))}
                onPriorityChange={(priority) => setDraft((current) => ({ ...current, priority }))}
                onTypeChange={(type) => setDraft((current) => ({ ...current, type }))}
                labels={{ status: t("remoteWorkspace.status"), priority: t("remoteWorkspace.priority"), type: t("remoteWorkspace.type") }}
                disabled={!canUpdateTickets}
              />
              <TextAreaField
                label={t("remoteWorkspace.description")}
                value={draft.description}
                onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))}
                maxLength={fieldLimits.ticketDescriptionMaxLength}
                disabled={!canUpdateTickets}
              />
              <Panel className="p-4">
                <div className="text-xs font-medium uppercase tracking-wide text-zinc-400">ActivityPub</div>
                <p className="mt-2 break-all text-sm text-zinc-600">{selectedTicket.ap_id}</p>
              </Panel>
            </div>
          </form>
          <div className="flex items-center justify-between gap-3 border-t border-zinc-200 px-6 py-4">
            <Button tone="danger" onClick={() => deleteTicket.mutate()} disabled={deleteTicket.isPending || !canDeleteTickets}>
              <Trash2 size={16} />
              {t("actions.delete")}
            </Button>
            <Button type="submit" form="remote-ticket-edit" tone="primary" disabled={updateTicket.isPending || !draft.title.trim() || !canUpdateTickets}>
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
            <Button type="submit" form="remote-create-ticket" tone="primary" disabled={createTicket.isPending || !createDraft.title.trim() || createDisabled}>
              {t("actions.create")}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          {createTicket.isError ? (
            <ErrorState title={t("remoteWorkspace.createFailed")} body={errorMessage(createTicket.error, t("remoteWorkspace.createFailedBody"))} />
          ) : null}
          {remoteProject && !canCreateTickets ? <ErrorState title={t("remoteWorkspace.readOnly")} body={t("remoteWorkspace.noCreatePermission")} /> : null}
          <TextField
            label={t("remoteWorkspace.ticketTitle")}
            value={createDraft.title}
            onChange={(event) => setCreateDraft((current) => ({ ...current, title: event.target.value }))}
            maxLength={fieldLimits.ticketTitleMaxLength}
            disabled={createDisabled}
            required
          />
          <TicketClassificationFields
            priority={createDraft.priority}
            type={createDraft.type}
            onPriorityChange={(priority) => setCreateDraft((current) => ({ ...current, priority }))}
            onTypeChange={(type) => setCreateDraft((current) => ({ ...current, type }))}
            labels={{ status: t("remoteWorkspace.status"), priority: t("remoteWorkspace.priority"), type: t("remoteWorkspace.type") }}
            disabled={createDisabled}
          />
          <TextAreaField
            label={t("remoteWorkspace.description")}
            value={createDraft.description}
            onChange={(event) => setCreateDraft((current) => ({ ...current, description: event.target.value }))}
            maxLength={fieldLimits.ticketDescriptionMaxLength}
            disabled={createDisabled}
          />
        </div>
      </Modal>
    </div>
  );
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Clock3, Flame, GitFork, ListChecks, Pencil, Plus, RefreshCw, Settings, Trash2, Truck } from "lucide-react";
import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { ProjectDeliveriesPanel } from "../features/projects/ProjectDeliveriesPanel";
import { ProjectSettingsPanel } from "../features/projects/ProjectSettingsPanel";
import { TicketBoard } from "../features/tickets/TicketBoard";
import { TicketDetailPanel } from "../features/tickets/TicketDetailPanel";
import { TicketFormModal } from "../features/tickets/TicketFormModal";
import { api, errorMessage, projectTicketEventsURL } from "../lib/api";
import { relativeDate } from "../lib/format";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import { useAuthStore } from "../store/auth";
import type { ID, Ticket, TicketEvent, TicketStatus } from "../types";
import { Button, ErrorState, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";

const ProjectGraph = lazy(() =>
  import("../features/graph/ProjectGraph").then((module) => ({ default: module.ProjectGraph })),
);

type ProjectView = "board" | "graph" | "deliveries" | "settings";

function viewFromPath(pathname: string): ProjectView {
  if (pathname.endsWith("/graph")) {
    return "graph";
  }
  if (pathname.endsWith("/deliveries")) {
    return "deliveries";
  }
  if (pathname.endsWith("/settings")) {
    return "settings";
  }
  return "board";
}

function tabClass(active: boolean): string {
  return [
    "focus-ring inline-flex h-9 items-center gap-2 rounded-full px-4 text-sm font-medium transition",
    active ? "bg-zinc-950 text-white shadow-sm" : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950",
  ].join(" ");
}

function SummaryItem({ icon, label, value }: { icon: ReactNode; label: string; value: number | string }) {
  return (
    <div className="inline-flex min-w-32 items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-3 py-1.5 text-sm">
      <span className="text-zinc-400">{icon}</span>
      <span className="font-semibold text-zinc-950">{value}</span>
      <span className="text-zinc-500">{label}</span>
    </div>
  );
}

function parseSSEMessage(raw: string): { event: string; data: string } | null {
  let event = "message";
  const data: string[] = [];
  for (const line of raw.split(/\r?\n/)) {
    if (line.startsWith("event:")) {
      event = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      data.push(line.slice("data:".length).trimStart());
    }
  }
  return data.length > 0 ? { event, data: data.join("\n") } : null;
}

function isTicketEvent(value: unknown): value is TicketEvent {
  if (!value || typeof value !== "object") {
    return false;
  }
  const event = value as Partial<TicketEvent>;
  return typeof event.type === "string" && event.type.startsWith("ticket.") && typeof event.project_id === "string";
}

export function ProjectWorkspace() {
  const { projectId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const token = useAuthStore((state) => state.token);
  const [createTicketOpen, setCreateTicketOpen] = useState(false);
  const [selectedTicketId, setSelectedTicketId] = useState<ID | null>(null);
  const [editProjectOpen, setEditProjectOpen] = useState(false);
  const [projectName, setProjectName] = useState("");
  const [projectDescription, setProjectDescription] = useState("");
  const activeProjectId = projectId || "";

  const view = viewFromPath(location.pathname);

  const project = useQuery({
    queryKey: queryKeys.project(activeProjectId),
    queryFn: () => api.getProject(activeProjectId),
    enabled: Boolean(projectId),
  });

  const tickets = useQuery({
    queryKey: queryKeys.tickets(activeProjectId),
    queryFn: () => api.listTickets(activeProjectId),
    enabled: Boolean(projectId),
  });

  const ticketStats = useMemo(() => {
    const currentTickets = tickets.data || [];
    return {
      total: currentTickets.length,
      active: currentTickets.filter((ticket) => ticket.status === "in_progress" || ticket.status === "review").length,
      urgent: currentTickets.filter((ticket) => ticket.priority === "urgent").length,
      done: currentTickets.filter((ticket) => ticket.status === "done").length,
    };
  }, [tickets.data]);

  const graph = useQuery({
    queryKey: queryKeys.graph(activeProjectId),
    queryFn: () => api.getProjectGraph(activeProjectId),
    enabled: Boolean(projectId) && view === "graph",
  });

  useEffect(() => {
    if (!activeProjectId || !token) {
      return;
    }

    const controller = new AbortController();
    const decoder = new TextDecoder();
    let reconnectTimer: number | undefined;

    function refreshTicketQueries(event: TicketEvent) {
      if (event.project_id !== activeProjectId) {
        return;
      }
      void queryClient.invalidateQueries({ queryKey: queryKeys.tickets(activeProjectId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.graph(activeProjectId) });
      if (event.ticket_id) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.ticket(event.ticket_id) });
      }
    }

    function handleFrame(frame: string) {
      const message = parseSSEMessage(frame);
      if (!message || message.event === "ready") {
        return;
      }
      try {
        const parsed: unknown = JSON.parse(message.data);
        if (isTicketEvent(parsed)) {
          refreshTicketQueries(parsed);
        }
      } catch {
        // Ignore malformed stream frames; reconnect handles transport failures.
      }
    }

    function scheduleReconnect() {
      if (!controller.signal.aborted) {
        reconnectTimer = window.setTimeout(connect, 3000);
      }
    }

    async function connect() {
      try {
        const response = await fetch(projectTicketEventsURL(activeProjectId), {
          headers: {
            Accept: "text/event-stream",
            Authorization: `Bearer ${token}`,
          },
          signal: controller.signal,
        });
        if (response.status === 401) {
          useAuthStore.getState().logout();
          return;
        }
        if (response.status === 403 || response.status === 404) {
          return;
        }
        if (!response.ok || !response.body) {
          throw new Error("ticket event stream unavailable");
        }

        const reader = response.body.getReader();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) {
            break;
          }
          buffer += decoder.decode(value, { stream: true });
          const frames = buffer.split(/\n\n/);
          buffer = frames.pop() || "";
          frames.forEach(handleFrame);
        }
        scheduleReconnect();
      } catch {
        scheduleReconnect();
      }
    }

    void connect();
    return () => {
      controller.abort();
      if (reconnectTimer) {
        window.clearTimeout(reconnectTimer);
      }
    };
  }, [activeProjectId, queryClient, token]);

  const updateTicketStatus = useMutation({
    mutationFn: ({ ticketId, status }: { ticketId: ID; status: TicketStatus }) => api.updateTicket(ticketId, { status }),
    onMutate: async ({ ticketId, status }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.tickets(activeProjectId) });
      const previous = queryClient.getQueryData<Ticket[]>(queryKeys.tickets(activeProjectId));
      queryClient.setQueryData<Ticket[]>(queryKeys.tickets(activeProjectId), (current = []) =>
        current.map((ticket) => (ticket.id === ticketId ? { ...ticket, status } : ticket)),
      );
      return { previous };
    },
    onError: (error, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.tickets(activeProjectId), context.previous);
      }
      toast.error(errorMessage(error, "Could not move ticket."));
    },
    onSettled: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(activeProjectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(activeProjectId) }),
      ]);
    },
  });

  const updateProject = useMutation({
    mutationFn: () => api.updateProject(activeProjectId, { name: projectName.trim(), description: projectDescription.trim() }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.project(activeProjectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.projects }),
      ]);
      setEditProjectOpen(false);
      toast.success("Project updated");
    },
  });

  const deleteProject = useMutation({
    mutationFn: () => api.deleteProject(activeProjectId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projects });
      navigate("/projects", { replace: true });
      toast.success("Project deleted");
    },
  });

  function openProjectEditor() {
    setProjectName(project.data?.name || "");
    setProjectDescription(project.data?.description || "");
    setEditProjectOpen(true);
  }

  function submitProject(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    updateProject.mutate();
  }

  return (
    <div className="space-y-5">
      {!projectId ? <ErrorState title="Missing project" body="The route did not include a project id." /> : null}
      {project.isLoading ? <LoadingState label="Loading project" /> : null}
      {project.isError ? (
        <ErrorState title="Could not load project" body={errorMessage(project.error, "Project request failed.")} />
      ) : null}

      {project.data ? (
        <>
          <div className="flex flex-col gap-4 rounded-3xl border border-zinc-200 bg-white p-5 shadow-sm xl:flex-row xl:items-start xl:justify-between">
            <div className="min-w-0">
              <div className="mb-2 inline-flex rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
                {project.data.handle}
              </div>
              <h1 className="truncate text-2xl font-semibold tracking-tight text-zinc-950">{project.data.name}</h1>
              <p className="mt-1 max-w-3xl text-sm text-zinc-500">{project.data.description || "No description"}</p>
              <div className="mt-4 flex flex-wrap gap-2">
                <SummaryItem icon={<ListChecks size={15} />} label="tickets" value={ticketStats.total} />
                <SummaryItem icon={<Clock3 size={15} />} label="active" value={ticketStats.active} />
                <SummaryItem icon={<Flame size={15} />} label="urgent" value={ticketStats.urgent} />
                <SummaryItem icon={<CheckCircle2 size={15} />} label="done" value={ticketStats.done} />
              </div>
              <p className="mt-3 text-xs text-zinc-400">Updated {relativeDate(project.data.updated_at)}</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => tickets.refetch()} disabled={tickets.isFetching}>
                <RefreshCw size={16} />
                Refresh
              </Button>
              <Button onClick={openProjectEditor}>
                <Pencil size={16} />
                Edit
              </Button>
              <Button tone="primary" onClick={() => setCreateTicketOpen(true)}>
                <Plus size={16} />
                Ticket
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="flex max-w-full overflow-x-auto rounded-full border border-zinc-200 bg-white p-1 shadow-sm">
              <Link to={`/projects/${activeProjectId}`} className={tabClass(view === "board")}>
                Board
              </Link>
              <Link to={`/projects/${activeProjectId}/graph`} className={tabClass(view === "graph")}>
                <GitFork size={16} />
                Graph
              </Link>
              <Link to={`/projects/${activeProjectId}/deliveries`} className={tabClass(view === "deliveries")}>
                <Truck size={16} />
                Deliveries
              </Link>
              <Link to={`/projects/${activeProjectId}/settings`} className={tabClass(view === "settings")}>
                <Settings size={16} />
                Settings
              </Link>
            </div>
            <div className="rounded-full border border-zinc-200 bg-white px-3 py-1 text-sm text-zinc-500 shadow-sm">
              {tickets.data?.length || 0} tickets
            </div>
          </div>

          {view === "board" && tickets.isLoading ? <LoadingState label="Loading tickets" /> : null}
          {view === "board" && tickets.isError ? (
            <ErrorState title="Could not load tickets" body={errorMessage(tickets.error, "Ticket list request failed.")} />
          ) : null}

          {view === "board" && tickets.data ? (
            <TicketBoard
              tickets={tickets.data}
              onOpenTicket={setSelectedTicketId}
              onMoveTicket={(ticketId, status) => updateTicketStatus.mutate({ ticketId, status })}
              emptyAction={
                <Button tone="primary" onClick={() => setCreateTicketOpen(true)}>
                  <Plus size={16} />
                  Create ticket
                </Button>
              }
            />
          ) : null}

          {view === "graph" ? (
            graph.isLoading ? (
              <LoadingState label="Loading graph" />
            ) : graph.isError ? (
              <ErrorState title="Could not load graph" body={errorMessage(graph.error, "Graph request failed.")} />
            ) : graph.data ? (
              <Suspense fallback={<LoadingState label="Loading graph view" />}>
                <ProjectGraph data={graph.data} tickets={tickets.data || []} onOpenTicket={setSelectedTicketId} />
              </Suspense>
            ) : null
          ) : null}

          {view === "deliveries" ? <ProjectDeliveriesPanel projectId={activeProjectId} /> : null}

          {view === "settings" ? <ProjectSettingsPanel project={project.data} tickets={tickets.data || []} /> : null}

          <TicketFormModal
            projectId={activeProjectId}
            tickets={tickets.data || []}
            open={createTicketOpen}
            onClose={() => setCreateTicketOpen(false)}
          />

          {selectedTicketId ? (
            <TicketDetailPanel
              projectId={activeProjectId}
              ticketId={selectedTicketId}
              tickets={tickets.data || []}
              onClose={() => setSelectedTicketId(null)}
            />
          ) : null}

          <Modal
            open={editProjectOpen}
            title="Edit Project"
            onClose={() => setEditProjectOpen(false)}
            formId="edit-project"
            onSubmit={submitProject}
            footer={
              <>
                <Button
                  tone="danger"
                  onClick={() => {
                    if (window.confirm("Delete this project?")) {
                      deleteProject.mutate();
                    }
                  }}
                  disabled={deleteProject.isPending}
                >
                  <Trash2 size={16} />
                  Delete
                </Button>
                <div className="flex flex-1 justify-end gap-2">
                  <Button onClick={() => setEditProjectOpen(false)}>Cancel</Button>
                  <Button type="submit" form="edit-project" tone="primary" disabled={updateProject.isPending || !projectName.trim()}>
                    Save
                  </Button>
                </div>
              </>
            }
          >
            <div className="grid gap-4">
              {updateProject.isError ? (
                <ErrorState title="Could not update project" body={errorMessage(updateProject.error, "Project update failed.")} />
              ) : null}
              {deleteProject.isError ? (
                <ErrorState title="Could not delete project" body={errorMessage(deleteProject.error, "Project delete failed.")} />
              ) : null}
              <TextField
                label="Name"
                value={projectName}
                onChange={(event) => setProjectName(event.target.value)}
                maxLength={fieldLimits.projectNameMaxLength}
                required
              />
              <TextAreaField
                label="Description"
                value={projectDescription}
                onChange={(event) => setProjectDescription(event.target.value)}
                maxLength={fieldLimits.projectDescriptionMaxLength}
              />
            </div>
          </Modal>
        </>
      ) : null}

      {!project.isLoading && !project.data && !project.isError ? (
        <Panel className="p-4 text-sm text-slate-500">Project not found.</Panel>
      ) : null}
    </div>
  );
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { GitFork, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { lazy, Suspense, useState } from "react";
import type { FormEvent } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { TicketBoard } from "../features/tickets/TicketBoard";
import { TicketDetailPanel } from "../features/tickets/TicketDetailPanel";
import { TicketFormModal } from "../features/tickets/TicketFormModal";
import { api, errorMessage } from "../lib/api";
import { relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import type { ID, Ticket, TicketStatus } from "../types";
import { Button, ErrorState, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";

const ProjectGraph = lazy(() =>
  import("../features/graph/ProjectGraph").then((module) => ({ default: module.ProjectGraph })),
);

function tabClass(active: boolean): string {
  return [
    "focus-ring inline-flex h-9 items-center gap-2 rounded-full px-4 text-sm font-medium transition",
    active ? "bg-zinc-950 text-white shadow-sm" : "text-zinc-500 hover:bg-zinc-100 hover:text-zinc-950",
  ].join(" ");
}

export function ProjectWorkspace() {
  const { projectId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [createTicketOpen, setCreateTicketOpen] = useState(false);
  const [selectedTicketId, setSelectedTicketId] = useState<ID | null>(null);
  const [editProjectOpen, setEditProjectOpen] = useState(false);
  const [projectName, setProjectName] = useState("");
  const [projectDescription, setProjectDescription] = useState("");
  const activeProjectId = projectId || "";

  const view = location.pathname.endsWith("/graph") ? "graph" : "board";

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

  const graph = useQuery({
    queryKey: queryKeys.graph(activeProjectId),
    queryFn: () => api.getProjectGraph(activeProjectId),
    enabled: Boolean(projectId) && view === "graph",
  });

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
              <p className="mt-2 text-xs text-zinc-400">Updated {relativeDate(project.data.updated_at)}</p>
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
            <div className="flex w-fit rounded-full border border-zinc-200 bg-white p-1 shadow-sm">
              <Link to={`/projects/${activeProjectId}`} className={tabClass(view === "board")}>
                Board
              </Link>
              <Link to={`/projects/${activeProjectId}/graph`} className={tabClass(view === "graph")}>
                <GitFork size={16} />
                Graph
              </Link>
            </div>
            <div className="rounded-full border border-zinc-200 bg-white px-3 py-1 text-sm text-zinc-500 shadow-sm">
              {tickets.data?.length || 0} tickets
            </div>
          </div>

          {tickets.isLoading ? <LoadingState label="Loading tickets" /> : null}
          {tickets.isError ? (
            <ErrorState title="Could not load tickets" body={errorMessage(tickets.error, "Ticket list request failed.")} />
          ) : null}

          {view === "board" && tickets.data ? (
            <TicketBoard
              tickets={tickets.data}
              onOpenTicket={setSelectedTicketId}
              onMoveTicket={(ticketId, status) => updateTicketStatus.mutate({ ticketId, status })}
            />
          ) : null}

          {view === "graph" ? (
            graph.isLoading ? (
              <LoadingState label="Loading graph" />
            ) : graph.isError ? (
              <ErrorState title="Could not load graph" body={errorMessage(graph.error, "Graph request failed.")} />
            ) : graph.data ? (
              <Suspense fallback={<LoadingState label="Loading graph view" />}>
                <ProjectGraph data={graph.data} />
              </Suspense>
            ) : null
          ) : null}

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
              <TextField label="Name" value={projectName} onChange={(event) => setProjectName(event.target.value)} required />
              <TextAreaField
                label="Description"
                value={projectDescription}
                onChange={(event) => setProjectDescription(event.target.value)}
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

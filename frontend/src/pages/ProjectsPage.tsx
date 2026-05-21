import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderKanban, Plus, RefreshCw } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import { Button, EmptyState, ErrorState, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const projects = useQuery({
    queryKey: queryKeys.projects,
    queryFn: api.listProjects,
  });

  const createProject = useMutation({
    mutationFn: api.createProject,
    onSuccess: async (project) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projects });
      setCreateOpen(false);
      setName("");
      setDescription("");
      navigate(`/projects/${project.id}`);
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    createProject.mutate({ name: name.trim(), description: description.trim() });
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-slate-950">Projects</h1>
          <p className="mt-1 text-sm text-slate-500">Create workspaces and jump into active boards.</p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => projects.refetch()} disabled={projects.isFetching}>
            <RefreshCw size={16} />
            Refresh
          </Button>
          <Button tone="primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} />
            Project
          </Button>
        </div>
      </div>

      {projects.isLoading ? <LoadingState label="Loading projects" /> : null}

      {projects.isError ? (
        <ErrorState title="Could not load projects" body={errorMessage(projects.error, "Project list request failed.")} />
      ) : null}

      {projects.data && projects.data.length === 0 ? (
        <EmptyState
          icon={<FolderKanban size={36} />}
          title="No projects yet"
          body="Create the first project and start shaping the board around real backend data."
          action={
            <Button tone="primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              Create project
            </Button>
          }
        />
      ) : null}

      {projects.data && projects.data.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {projects.data.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`} className="focus-ring rounded-lg">
              <Panel className="h-full p-4 transition hover:border-cyan-300 hover:shadow-sm">
                <div className="flex items-start gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-md bg-cyan-50 text-cyan-700">
                    <FolderKanban size={20} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <h2 className="truncate text-base font-semibold text-slate-950">{project.name}</h2>
                    <p className="mt-1 line-clamp-2 min-h-10 text-sm text-slate-500">
                      {project.description || "No description"}
                    </p>
                  </div>
                </div>
                <div className="mt-4 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-500">
                  <span className="truncate">{project.handle}</span>
                  <span>{relativeDate(project.updated_at)}</span>
                </div>
              </Panel>
            </Link>
          ))}
        </div>
      ) : null}

      <Modal
        open={createOpen}
        title="Create Project"
        onClose={() => setCreateOpen(false)}
        formId="create-project"
        onSubmit={submit}
        footer={
          <>
            <Button onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button type="submit" form="create-project" tone="primary" disabled={createProject.isPending || !name.trim()}>
              Create
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          {createProject.isError ? (
            <ErrorState title="Could not create project" body={errorMessage(createProject.error, "Project creation failed.")} />
          ) : null}
          <TextField label="Name" value={name} onChange={(event) => setName(event.target.value)} required />
          <TextAreaField
            label="Description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="What this project is responsible for"
          />
        </div>
      </Modal>
    </div>
  );
}

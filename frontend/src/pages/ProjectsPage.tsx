import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, FolderKanban, Plus, RefreshCw } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { relativeDate } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import { Button, EmptyState, ErrorState, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { t } = useI18n();
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
    <div className="space-y-6">
      <div className="flex flex-col gap-4 rounded-3xl border border-zinc-200 bg-white p-5 shadow-sm md:flex-row md:items-center md:justify-between">
        <div>
          <div className="mb-2 inline-flex rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
            {t("projects.badge")}
          </div>
          <h1 className="text-3xl font-semibold tracking-tight text-zinc-950">{t("projects.title")}</h1>
          <p className="mt-1 text-sm text-zinc-500">{t("projects.subtitle")}</p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => projects.refetch()} disabled={projects.isFetching}>
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
          <Button tone="primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} />
            {t("projects.project")}
          </Button>
        </div>
      </div>

      {projects.isLoading ? <LoadingState label={t("projects.loading")} /> : null}

      {projects.isError ? (
        <ErrorState title={t("projects.loadFailed")} body={errorMessage(projects.error, t("projects.loadFailedBody"))} />
      ) : null}

      {projects.data && projects.data.length === 0 ? (
        <EmptyState
          icon={<FolderKanban size={36} />}
          title={t("projects.emptyTitle")}
          body={t("projects.emptyBody")}
          action={
            <Button tone="primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              {t("projects.createProject")}
            </Button>
          }
        />
      ) : null}

      {projects.data && projects.data.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.data.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`} className="group focus-ring rounded-2xl">
              <Panel className="h-full overflow-hidden p-4 transition hover:-translate-y-0.5 hover:border-zinc-300 hover:shadow-md">
                <div className="flex items-start gap-3">
                  <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-zinc-200 bg-zinc-950 text-white shadow-sm">
                    <FolderKanban size={20} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <h2 className="truncate text-base font-semibold text-zinc-950">{project.name}</h2>
                      <ArrowUpRight size={16} className="mt-0.5 shrink-0 text-zinc-300 transition group-hover:text-zinc-950" />
                    </div>
                    <p className="mt-1 line-clamp-2 min-h-10 text-sm text-zinc-500">
                      {project.description || t("common.noDescription")}
                    </p>
                  </div>
                </div>
                <div className="mt-4 flex items-center justify-between border-t border-zinc-100 pt-3 text-xs text-zinc-500">
                  <span className="truncate rounded-full border border-zinc-200 px-2 py-0.5">{project.handle}</span>
                  <span>{relativeDate(project.updated_at)}</span>
                </div>
              </Panel>
            </Link>
          ))}
        </div>
      ) : null}

      <Modal
        open={createOpen}
        title={t("projects.createTitle")}
        onClose={() => setCreateOpen(false)}
        formId="create-project"
        onSubmit={submit}
        footer={
          <>
            <Button onClick={() => setCreateOpen(false)}>{t("actions.cancel")}</Button>
            <Button type="submit" form="create-project" tone="primary" disabled={createProject.isPending || !name.trim()}>
              {t("actions.create")}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          {createProject.isError ? (
            <ErrorState title={t("projects.createFailed")} body={errorMessage(createProject.error, t("projects.createFailedBody"))} />
          ) : null}
          <TextField
            label={t("projects.name")}
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={fieldLimits.projectNameMaxLength}
            required
          />
          <TextAreaField
            label={t("projects.description")}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            maxLength={fieldLimits.projectDescriptionMaxLength}
            placeholder={t("projects.descriptionPlaceholder")}
          />
        </div>
      </Modal>
    </div>
  );
}

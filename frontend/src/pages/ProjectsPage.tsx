import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, FolderKanban, Network, Plus, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, errorMessage } from "../lib/api";
import { relativeDate } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { fieldLimits } from "../lib/limits";
import { queryKeys } from "../lib/queryKeys";
import { Badge, Button, EmptyState, ErrorState, LoadingState, Modal, Panel, TextAreaField, TextField } from "../components/ui";

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
  const remoteFollows = useQuery({
    queryKey: queryKeys.personalFederationFollows("accepted"),
    queryFn: () => api.listPersonalFederationFollows({ state: "accepted" }),
  });

  const capabilities = useQuery({
    queryKey: queryKeys.instanceCapabilities,
    queryFn: api.getInstanceCapabilities,
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

  const canCreateProjects = capabilities.data?.can_create_projects ?? false;
  const localProjects = projects.data || [];
  const remoteProjects = (remoteFollows.data || []).filter((follow) => follow.actor_type === "Group");
  const hasAnyProjects = localProjects.length > 0 || remoteProjects.length > 0;

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canCreateProjects) {
      return;
    }
    createProject.mutate({ name: name.trim(), description: description.trim() });
  }

  useEffect(() => {
    if (!canCreateProjects && createOpen) {
      setCreateOpen(false);
    }
  }, [canCreateProjects, createOpen]);

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
          {canCreateProjects ? (
            <Button tone="primary" onClick={() => setCreateOpen(true)}>
              <Plus size={16} />
              {t("projects.project")}
            </Button>
          ) : null}
        </div>
      </div>

      {projects.isLoading ? <LoadingState label={t("projects.loading")} /> : null}

      {projects.isError ? (
        <ErrorState title={t("projects.loadFailed")} body={errorMessage(projects.error, t("projects.loadFailedBody"))} />
      ) : null}

      {capabilities.isError ? (
        <ErrorState title={t("projects.capabilitiesFailed")} body={errorMessage(capabilities.error, t("projects.capabilitiesFailedBody"))} />
      ) : null}

      {remoteFollows.isError ? (
        <ErrorState title={t("projects.remoteLoadFailed")} body={errorMessage(remoteFollows.error, t("projects.remoteLoadFailedBody"))} />
      ) : null}

      {projects.data && !remoteFollows.isLoading && !hasAnyProjects ? (
        <EmptyState
          icon={<FolderKanban size={36} />}
          title={t("projects.emptyTitle")}
          body={canCreateProjects ? t("projects.emptyBody") : t("projects.emptyRestrictedBody")}
          action={
            canCreateProjects ? (
              <Button tone="primary" onClick={() => setCreateOpen(true)}>
                <Plus size={16} />
                {t("projects.createProject")}
              </Button>
            ) : null
          }
        />
      ) : null}

      {localProjects.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {localProjects.map((project) => (
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

      {remoteProjects.length > 0 ? (
        <Panel className="overflow-hidden">
          <div className="flex flex-col gap-2 border-b border-zinc-100 p-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
                <Network size={18} />
                {t("projects.remoteTitle")}
              </h2>
              <p className="mt-1 text-sm text-zinc-500">{t("projects.remoteSubtitle")}</p>
            </div>
            <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{t("common.shown", { count: remoteProjects.length })}</Badge>
          </div>
          <div className="grid gap-4 p-4 md:grid-cols-2 xl:grid-cols-3">
            {remoteProjects.map((project) => (
              <div key={project.actor_id} className="rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm">
                <div className="flex items-start gap-3">
                  <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl border border-zinc-200 bg-zinc-50 text-zinc-700 shadow-sm">
                    <Network size={20} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <h3 className="truncate text-base font-semibold text-zinc-950">{project.name || project.handle || project.actor_ap_id}</h3>
                      <Badge className="border-zinc-950 bg-zinc-950 text-white">{t("projects.remoteBadge")}</Badge>
                    </div>
                    <p className="mt-1 line-clamp-2 min-h-10 text-sm text-zinc-500">
                      {project.summary || t("projects.remoteProjectBody")}
                    </p>
                  </div>
                </div>
                <div className="mt-4 border-t border-zinc-100 pt-3">
                  <p className="truncate text-xs text-zinc-500">{project.actor_ap_id}</p>
                  <div className="mt-3 flex items-center justify-between gap-2">
                    <span className="truncate rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">{project.handle}</span>
                    <Link
                      to="/federation"
                      className="focus-ring inline-flex h-8 items-center justify-center gap-1.5 rounded-full border border-zinc-200 bg-white px-3 text-xs font-medium text-zinc-800 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
                    >
                      {t("projects.viewFederation")}
                      <ArrowUpRight size={14} />
                    </Link>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </Panel>
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
            <Button type="submit" form="create-project" tone="primary" disabled={createProject.isPending || !name.trim() || !canCreateProjects}>
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

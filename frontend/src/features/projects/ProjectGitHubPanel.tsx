import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Clock3, Download, FileJson, Github, GitBranch, History, Link2, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type { GitHubRepository, ID } from "../../types";
import { MetricPill } from "./ProjectAdminOverview";
import { downloadText, githubCommitsToCSV, githubRepositoriesToCSV } from "./projectSettingsExports";

function parseRepositoryRef(value: string): { owner: string; name: string } | null {
  const normalized = value.trim().replace(/^https:\/\/github\.com\//i, "").replace(/\.git$/i, "");
  const [owner, name] = normalized.split("/");
  if (!owner?.trim() || !name?.trim()) {
    return null;
  }
  return { owner: owner.trim(), name: name.trim() };
}

function commitTitle(message: string): string {
  return message.split(/\r?\n/)[0] || "Commit";
}

export function ProjectGitHubPanel({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [repoRef, setRepoRef] = useState("");
  const [commitSearch, setCommitSearch] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const repositories = useQuery({
    queryKey: queryKeys.githubRepositories(projectId),
    queryFn: () => api.listGitHubRepositories(projectId),
  });

  const commitSearchValue = commitSearch.trim();
  const commits = useQuery({
    queryKey: queryKeys.projectGitHubCommits(projectId, commitSearchValue || "recent"),
    queryFn: () => api.listProjectGitHubCommits(projectId, { q: commitSearchValue || undefined, limit: 50 }),
  });

  async function refreshGitHubData() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.githubRepositories(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projectGitHubCommitsScope(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.ticketGitHubCommitsScope }),
    ]);
  }

  const linkRepository = useMutation({
    mutationFn: () => {
      const parsed = parseRepositoryRef(repoRef);
      if (!parsed) {
        throw new Error("Use owner/repository.");
      }
      return api.linkGitHubRepository(projectId, parsed);
    },
    onSuccess: async () => {
      await refreshGitHubData();
      setRepoRef("");
      setFormError(null);
      toast.success("GitHub repository linked");
    },
    onError: (error) => setFormError(errorMessage(error, "Repository link failed.")),
  });

  const syncRepository = useMutation({
    mutationFn: (repositoryId: ID) => api.syncGitHubRepository(projectId, repositoryId),
    onSuccess: async (result) => {
      await refreshGitHubData();
      toast.success(`GitHub sync imported ${result.imported} commits and linked ${result.linked}.`);
    },
    onError: (error) => toast.error(errorMessage(error, "GitHub sync failed.")),
  });

  const deleteRepository = useMutation({
    mutationFn: (repositoryId: ID) => api.deleteGitHubRepository(projectId, repositoryId),
    onSuccess: async () => {
      await refreshGitHubData();
      toast.success("GitHub repository removed");
    },
    onError: (error) => toast.error(errorMessage(error, "Repository removal failed.")),
  });

  function submitRepository(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    linkRepository.mutate();
  }

  const repoRows = repositories.data || [];
  const commitRows = commits.data || [];
  const totalCommits = repoRows.reduce((sum, repo) => sum + repo.commit_count, 0);
  const linkedCommits = repoRows.reduce((sum, repo) => sum + repo.linked_commit_count, 0);
  const manualLinks = repoRows.reduce((sum, repo) => sum + repo.manual_link_count, 0);
  const latestWebhook = repoRows
    .map((repo) => repo.last_webhook_at)
    .filter(Boolean)
    .sort()
    .at(-1);

  function exportGitHubJSON() {
    const report = { generated_at: new Date().toISOString(), repositories: repoRows, commits: commitRows };
    downloadText(`github-integration-${projectId}.json`, "application/json", `${JSON.stringify(report, null, 2)}\n`);
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950"><Github size={17} />GitHub Repositories</h2>
          <p className="mt-1 text-sm text-zinc-500">Project repositories, imported commits, ticket references, and sync health.</p>
        </div>
        <form className="flex flex-col gap-2 sm:flex-row" onSubmit={submitRepository}>
          <TextField label="Repository" className="min-w-64" placeholder="owner/repository" value={repoRef} onChange={(event) => setRepoRef(event.target.value)} />
          <Button type="submit" tone="primary" className="self-end" disabled={linkRepository.isPending || !repoRef.trim()}><Plus size={16} />Link</Button>
        </form>
      </div>

      <div className="grid gap-3 border-t border-zinc-100 p-4 sm:grid-cols-2 xl:grid-cols-5">
        <MetricPill icon={<Github size={14} />} label="Repos" value={repositories.isLoading ? "..." : repoRows.length} />
        <MetricPill icon={<History size={14} />} label="Commits" value={repositories.isLoading ? "..." : totalCommits} />
        <MetricPill icon={<Link2 size={14} />} label="Linked" value={repositories.isLoading ? "..." : linkedCommits} />
        <MetricPill icon={<Pencil size={14} />} label="Manual" value={repositories.isLoading ? "..." : manualLinks} />
        <MetricPill icon={<Clock3 size={14} />} label="Webhook" value={latestWebhook ? relativeDate(latestWebhook) : "none"} />
      </div>

      {formError ? <div className="border-t border-zinc-100 p-4"><ErrorState title="Could not link repository" body={formError} /></div> : null}

      <div className="border-t border-zinc-100 p-4">
        {repositories.isLoading ? <LoadingState label="Loading GitHub repositories" /> : null}
        {repositories.isError ? <ErrorState title="Could not load GitHub repositories" body={errorMessage(repositories.error, "Repository request failed.")} /> : null}
        {!repositories.isLoading && !repositories.isError && repoRows.length === 0 ? <div className="rounded-xl border border-dashed border-zinc-300 px-4 py-6 text-sm text-zinc-500">No GitHub repositories linked.</div> : null}
        <div className="grid gap-2">
          {repoRows.map((repo: GitHubRepository) => {
            const syncing = syncRepository.isPending && syncRepository.variables === repo.id;
            const removing = deleteRepository.isPending && deleteRepository.variables === repo.id;
            return (
              <div key={repo.id} className="rounded-xl border border-zinc-200 px-3 py-3">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0">
                    <a className="inline-flex max-w-full items-center gap-2 truncate text-sm font-semibold text-zinc-950 underline-offset-4 hover:underline" href={repo.html_url} target="_blank" rel="noreferrer"><Github size={15} className="shrink-0" /><span className="truncate">{repo.full_name}</span></a>
                    <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-zinc-500">
                      <span className="inline-flex items-center gap-1"><GitBranch size={13} />{repo.default_branch || "default"}</span>
                      <span>{repo.last_synced_at ? `Synced ${relativeDate(repo.last_synced_at)}` : "Never synced"}</span>
                      <span>{repo.last_webhook_at ? `Webhook ${relativeDate(repo.last_webhook_at)}` : "No webhook"}</span>
                      <span>{repo.commit_count} commits</span><span>{repo.linked_commit_count} linked</span>
                    </div>
                    {repo.last_sync_error ? <div className="mt-2 flex items-center gap-1 text-xs text-red-600"><AlertTriangle size={13} /><span className="line-clamp-1">{repo.last_sync_error}</span></div> : null}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button onClick={() => syncRepository.mutate(repo.id)} disabled={syncing || removing}><RefreshCw size={16} />{syncing ? "Syncing" : "Sync"}</Button>
                    <Button tone="danger" onClick={() => { if (window.confirm(`Remove ${repo.full_name}?`)) deleteRepository.mutate(repo.id); }} disabled={syncing || removing}><Trash2 size={16} />Remove</Button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="border-t border-zinc-100 p-4">
        <div className="mb-3 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div><h3 className="text-sm font-semibold text-zinc-950">Recent Commits</h3><p className="mt-1 text-xs text-zinc-500">Imported commit activity across linked repositories.</p></div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <TextField label="Search" className="min-w-64" placeholder="message, SHA, author" value={commitSearch} onChange={(event) => setCommitSearch(event.target.value)} />
            <Button className="self-end" onClick={() => downloadText(`github-repositories-${projectId}.csv`, "text/csv", `${githubRepositoriesToCSV(repoRows)}\n`)} disabled={repoRows.length === 0}><Download size={15} />Repos CSV</Button>
            <Button className="self-end" onClick={() => downloadText(`github-commits-${projectId}.csv`, "text/csv", `${githubCommitsToCSV(commitRows)}\n`)} disabled={commitRows.length === 0}><Download size={15} />Commits CSV</Button>
            <Button className="self-end" onClick={exportGitHubJSON}><FileJson size={15} />JSON</Button>
          </div>
        </div>

        {commits.isLoading ? <LoadingState label="Loading GitHub commits" /> : null}
        {commits.isError ? <ErrorState title="Could not load GitHub commits" body={errorMessage(commits.error, "Commit request failed.")} /> : null}
        {!commits.isLoading && !commits.isError && commitRows.length === 0 ? <div className="rounded-xl border border-dashed border-zinc-300 px-4 py-6 text-sm text-zinc-500">No imported commits found.</div> : null}
        <div className="divide-y divide-zinc-100 rounded-xl border border-zinc-200">
          {commitRows.map((commit) => (
            <div key={commit.id} className="grid gap-3 p-3 lg:grid-cols-[1fr_auto] lg:items-center">
              <div className="min-w-0">
                <a className="line-clamp-1 text-sm font-medium text-zinc-950 underline-offset-4 hover:underline" href={commit.html_url} target="_blank" rel="noreferrer">{commitTitle(commit.message)}</a>
                <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-zinc-500"><span>{commit.repository_full_name}</span><span>{commit.author_name || "Unknown author"}</span><span>{commit.authored_at ? relativeDate(commit.authored_at) : "unknown date"}</span></div>
              </div>
              <div className="flex flex-wrap items-center gap-2 lg:justify-end"><code className="rounded-md bg-zinc-50 px-1.5 py-0.5 text-xs text-zinc-600">{commit.short_sha}</code><span className="rounded-full border border-zinc-200 bg-white px-2 py-0.5 text-xs text-zinc-600">{commit.ticket_ids.length} tickets</span></div>
            </div>
          ))}
        </div>
      </div>
    </Panel>
  );
}

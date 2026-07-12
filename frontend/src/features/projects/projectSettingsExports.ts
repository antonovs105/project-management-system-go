import type { GitHubCommit, GitHubRepository, ProjectInviteInspection, ProjectMember, Ticket } from "../../types";

export function safeFilePart(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "project";
}

function csvCell(value: string | number | boolean | null | undefined): string {
  let raw = value === null || value === undefined ? "" : String(value);
  if (typeof value === "string" && /^[=+\-@\t\r]/.test(raw)) {
    raw = `'${raw}`;
  }
  return `"${raw.replace(/"/g, '""')}"`;
}

export function downloadText(filename: string, mimeType: string, content: string) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

export function ticketsToCSV(tickets: Ticket[]): string {
  const header = ["id", "title", "status", "priority", "type", "is_resolved", "parent_id", "assignee_id", "reporter_id", "created_at", "updated_at"];
  const rows = tickets.map((ticket) => [
    ticket.id,
    ticket.title,
    ticket.status,
    ticket.priority,
    ticket.type,
    ticket.is_resolved,
    ticket.parent_id,
    ticket.assignee_id,
    ticket.reporter_id,
    ticket.created_at,
    ticket.updated_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

export function membersToCSV(members: ProjectMember[]): string {
  const header = ["user_id", "username", "email", "handle", "name", "is_remote", "role_id", "role", "role_name", "created_at"];
  const rows = members.map((member) => [
    member.user_id,
    member.username,
    member.email,
    member.handle,
    member.name,
    String(member.is_remote),
    member.role_id,
    member.role,
    member.role_name,
    member.created_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

export function invitesToCSV(invites: ProjectInviteInspection[]): string {
  const header = [
    "id", "status", "invitee_actor_id", "invitee_username", "invitee_email", "invitee_handle", "role_id", "role", "role_name",
    "inviter_actor_id", "inviter_username", "created_at", "updated_at",
  ];
  const rows = invites.map((invite) => [
    invite.id,
    invite.status,
    invite.invitee_actor_id,
    invite.invitee_username,
    invite.invitee_email,
    invite.invitee_handle,
    invite.role_id,
    invite.role,
    invite.role_name,
    invite.inviter_actor_id,
    invite.inviter_username,
    invite.created_at,
    invite.updated_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

export function githubRepositoriesToCSV(repositories: GitHubRepository[]): string {
  const header = [
    "id", "project_id", "full_name", "html_url", "default_branch", "last_synced_at", "last_webhook_at", "last_sync_error",
    "commit_count", "linked_commit_count", "manual_link_count", "created_at", "updated_at",
  ];
  const rows = repositories.map((repository) => [
    repository.id,
    repository.project_id,
    repository.full_name,
    repository.html_url,
    repository.default_branch,
    repository.last_synced_at,
    repository.last_webhook_at,
    repository.last_sync_error,
    repository.commit_count,
    repository.linked_commit_count,
    repository.manual_link_count,
    repository.created_at,
    repository.updated_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

export function githubCommitsToCSV(commits: GitHubCommit[]): string {
  const header = [
    "id", "repository_id", "repository_full_name", "sha", "short_sha", "message", "author_name", "author_email", "authored_at",
    "html_url", "ticket_ids", "link_source", "created_at", "updated_at",
  ];
  const rows = commits.map((commit) => [
    commit.id,
    commit.repository_id,
    commit.repository_full_name,
    commit.sha,
    commit.short_sha,
    commit.message,
    commit.author_name,
    commit.author_email,
    commit.authored_at,
    commit.html_url,
    commit.ticket_ids.join(" "),
    commit.link_source,
    commit.created_at,
    commit.updated_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

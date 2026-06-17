import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  BarChart3,
  CheckCircle2,
  Clock3,
  Copy,
  Download,
  FileJson,
  Flame,
  Github,
  GitBranch,
  Link2,
  History,
  RefreshCw,
  ListChecks,
  Mail,
  Pencil,
  Plus,
  Send,
  Shield,
  Trash2,
  UserMinus,
  UserPlus,
  Users,
} from "lucide-react";
import { useMemo, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Modal, Panel, SelectField, TextAreaField, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { projectPermissionGroups, ticketPriorities, ticketStatuses } from "../../lib/constants";
import { compactId, initials, relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type {
  ID,
  GitHubCommit,
  GitHubRepository,
  Project,
  ProjectDeliverySummary,
  ProjectInvite,
  ProjectInviteInspection,
  ProjectMember,
  ProjectPermission,
  ProjectRole,
  Ticket,
} from "../../types";

function allProjectPermissions(): ProjectPermission[] {
  return projectPermissionGroups.flatMap((group) => group.permissions.map((permission) => permission.id));
}

function PermissionPicker({
  value,
  onChange,
}: {
  value: ProjectPermission[];
  onChange: (permissions: ProjectPermission[]) => void;
}) {
  const selected = useMemo(() => new Set(value), [value]);

  function toggle(permission: ProjectPermission) {
    const next = new Set(selected);
    if (next.has(permission)) {
      next.delete(permission);
    } else {
      next.add(permission);
    }
    onChange(allProjectPermissions().filter((item) => next.has(item)));
  }

  return (
    <div className="grid gap-3">
      {projectPermissionGroups.map((group) => (
        <div key={group.group} className="rounded-xl border border-zinc-200 p-3">
          <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-400">{group.group}</div>
          <div className="grid gap-2 sm:grid-cols-2">
            {group.permissions.map((permission) => (
              <label key={permission.id} className="flex items-center gap-2 text-sm text-zinc-700">
                <input
                  type="checkbox"
                  className="h-4 w-4 rounded border-zinc-300 accent-zinc-950"
                  checked={selected.has(permission.id)}
                  onChange={() => toggle(permission.id)}
                />
                <span>{permission.label}</span>
              </label>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function permissionsSummary(role: ProjectRole): string {
  if (role.permissions.length === allProjectPermissions().length) {
    return "All permissions";
  }
  if (role.permissions.length === 0) {
    return "No permissions";
  }
  return `${role.permissions.length} permissions`;
}

function percent(count: number, total: number): number {
  if (total === 0) {
    return 0;
  }
  return Math.round((count / total) * 100);
}

function safeFilePart(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "project";
}

function csvCell(value: string | number | boolean | null | undefined): string {
  const raw = value === null || value === undefined ? "" : String(value);
  return `"${raw.replace(/"/g, '""')}"`;
}

function downloadText(filename: string, mimeType: string, content: string) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function ticketsToCSV(tickets: Ticket[]): string {
  const header = [
    "id",
    "title",
    "status",
    "priority",
    "type",
    "is_resolved",
    "parent_id",
    "assignee_id",
    "reporter_id",
    "created_at",
    "updated_at",
  ];
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

function membersToCSV(members: ProjectMember[]): string {
  const header = ["user_id", "username", "email", "handle", "name", "role_id", "role", "role_name", "created_at"];
  const rows = members.map((member) => [
    member.user_id,
    member.username,
    member.email,
    member.handle,
    member.name,
    member.role_id,
    member.role,
    member.role_name,
    member.created_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

function invitesToCSV(invites: ProjectInviteInspection[]): string {
  const header = [
    "id",
    "status",
    "invitee_actor_id",
    "invitee_username",
    "invitee_email",
    "invitee_handle",
    "role_id",
    "role",
    "role_name",
    "inviter_actor_id",
    "inviter_username",
    "created_at",
    "updated_at",
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

function githubRepositoriesToCSV(repositories: GitHubRepository[]): string {
  const header = [
    "id",
    "project_id",
    "full_name",
    "html_url",
    "default_branch",
    "last_synced_at",
    "last_webhook_at",
    "last_sync_error",
    "commit_count",
    "linked_commit_count",
    "manual_link_count",
    "created_at",
    "updated_at",
  ];
  const rows = repositories.map((repo) => [
    repo.id,
    repo.project_id,
    repo.full_name,
    repo.html_url,
    repo.default_branch,
    repo.last_synced_at,
    repo.last_webhook_at,
    repo.last_sync_error,
    repo.commit_count,
    repo.linked_commit_count,
    repo.manual_link_count,
    repo.created_at,
    repo.updated_at,
  ]);
  return [header, ...rows].map((row) => row.map(csvCell).join(",")).join("\n");
}

function githubCommitsToCSV(commits: GitHubCommit[]): string {
  const header = [
    "id",
    "repository_id",
    "repository_full_name",
    "sha",
    "short_sha",
    "message",
    "author_name",
    "author_email",
    "authored_at",
    "html_url",
    "ticket_ids",
    "link_source",
    "created_at",
    "updated_at",
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

function actorTitle(name: string, username: string, handle: string): string {
  return name || username || handle || "Unknown actor";
}

function actorSubtitle(handle: string, email?: string): string {
  if (email) {
    return `${handle} / ${email}`;
  }
  return handle || "No handle";
}

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

function InviteStatusBadge({ status }: { status: ProjectInvite["status"] }) {
  const tone =
    status === "pending"
      ? "border-zinc-300 bg-white text-zinc-700"
      : status === "accepted"
        ? "border-emerald-200 bg-emerald-50 text-emerald-700"
        : status === "rejected"
          ? "border-zinc-200 bg-zinc-50 text-zinc-500"
          : "border-red-200 bg-red-50 text-red-700";
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium ${tone}`}>{status}</span>;
}

function PersonMark({ label }: { label: string }) {
  return (
    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-zinc-50 text-xs font-semibold text-zinc-700">
      {initials(label)}
    </div>
  );
}

function MetricPill({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value: number | string;
}) {
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50 px-3 py-3">
      <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-zinc-400">
        {icon}
        {label}
      </div>
      <div className="mt-2 text-2xl font-semibold text-zinc-950">{value}</div>
    </div>
  );
}

function DistributionList({ title, rows, total }: { title: string; rows: Array<{ id: string; label: string; count: number }>; total: number }) {
  return (
    <div className="rounded-2xl border border-zinc-200 p-4">
      <h3 className="text-sm font-semibold text-zinc-950">{title}</h3>
      <div className="mt-4 grid gap-3">
        {rows.map((row) => (
          <div key={row.id} className="grid gap-1.5">
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="text-zinc-600">{row.label}</span>
              <span className="font-medium text-zinc-950">{row.count}</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-zinc-100">
              <div className="h-full rounded-full bg-zinc-950" style={{ width: `${percent(row.count, total)}%` }} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ProjectAdminOverview({ project, tickets }: { project: Project; tickets: Ticket[] }) {
  const roles = useQuery({
    queryKey: queryKeys.projectRoles(project.id),
    queryFn: () => api.listProjectRoles(project.id),
  });
  const deliverySummary = useQuery({
    queryKey: queryKeys.projectDeliverySummary(project.id),
    queryFn: () => api.getProjectDeliverySummary(project.id),
  });

  const statusRows = ticketStatuses.map((status) => ({
    ...status,
    count: tickets.filter((ticket) => ticket.status === status.id).length,
  }));
  const priorityRows = ticketPriorities.map((priority) => ({
    ...priority,
    count: tickets.filter((ticket) => ticket.priority === priority.id).length,
  }));
  const activeTickets = tickets.filter((ticket) => ticket.status === "in_progress" || ticket.status === "review").length;
  const urgentTickets = tickets.filter((ticket) => ticket.priority === "urgent").length;
  const doneTickets = tickets.filter((ticket) => ticket.status === "done").length;
  const customRoles = roles.data?.filter((role) => !role.is_system).length || 0;
  const delivery = deliverySummary.data;

  function exportJSON() {
    const report: {
      project: Project;
      generated_at: string;
      ticket_summary: Record<string, number>;
      status_distribution: typeof statusRows;
      priority_distribution: typeof priorityRows;
      roles: ProjectRole[];
      delivery_summary: ProjectDeliverySummary | null;
      tickets: Ticket[];
    } = {
      project,
      generated_at: new Date().toISOString(),
      ticket_summary: {
        total: tickets.length,
        active: activeTickets,
        urgent: urgentTickets,
        done: doneTickets,
        unresolved: tickets.length - doneTickets,
      },
      status_distribution: statusRows,
      priority_distribution: priorityRows,
      roles: roles.data || [],
      delivery_summary: delivery || null,
      tickets,
    };
    downloadText(`${safeFilePart(project.name)}-report.json`, "application/json", `${JSON.stringify(report, null, 2)}\n`);
  }

  function exportCSV() {
    downloadText(`${safeFilePart(project.name)}-tickets.csv`, "text/csv", `${ticketsToCSV(tickets)}\n`);
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-start md:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
            <BarChart3 size={17} />
            Administration Overview
          </h2>
          <p className="mt-1 text-sm text-zinc-500">Operational snapshot and exports for project administrators.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={exportCSV} disabled={tickets.length === 0}>
            <Download size={16} />
            CSV
          </Button>
          <Button onClick={exportJSON}>
            <FileJson size={16} />
            JSON
          </Button>
        </div>
      </div>

      <div className="grid gap-3 border-t border-zinc-100 p-4 sm:grid-cols-2 xl:grid-cols-5">
        <MetricPill icon={<ListChecks size={14} />} label="Tickets" value={tickets.length} />
        <MetricPill icon={<Clock3 size={14} />} label="Active" value={activeTickets} />
        <MetricPill icon={<Flame size={14} />} label="Urgent" value={urgentTickets} />
        <MetricPill icon={<CheckCircle2 size={14} />} label="Done" value={doneTickets} />
        <MetricPill icon={<Shield size={14} />} label="Custom Roles" value={roles.isLoading ? "..." : customRoles} />
      </div>

      <div className="grid gap-4 border-t border-zinc-100 p-4 xl:grid-cols-[1fr_1fr_0.8fr]">
        <DistributionList title="Ticket Status" rows={statusRows} total={tickets.length} />
        <DistributionList title="Priority Mix" rows={priorityRows} total={tickets.length} />
        <div className="rounded-2xl border border-zinc-200 p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
            <Activity size={15} />
            Delivery Health
          </h3>
          {deliverySummary.isLoading ? <div className="mt-4 text-sm text-zinc-500">Loading delivery summary</div> : null}
          {deliverySummary.isError ? (
            <div className="mt-4">
              <ErrorState title="Could not load delivery summary" body={errorMessage(deliverySummary.error, "Delivery summary failed.")} />
            </div>
          ) : null}
          {delivery ? (
            <div className="mt-4 grid grid-cols-2 gap-2 text-sm">
              <div className="rounded-xl bg-zinc-50 p-3">
                <div className="text-xs text-zinc-400">Total</div>
                <div className="mt-1 font-semibold text-zinc-950">{delivery.total}</div>
              </div>
              <div className="rounded-xl bg-zinc-50 p-3">
                <div className="text-xs text-zinc-400">Failed</div>
                <div className="mt-1 font-semibold text-zinc-950">{delivery.failed + delivery.dead}</div>
              </div>
              <div className="rounded-xl bg-zinc-50 p-3">
                <div className="text-xs text-zinc-400">Retryable</div>
                <div className="mt-1 font-semibold text-zinc-950">{delivery.retryable}</div>
              </div>
              <div className="rounded-xl bg-zinc-50 p-3">
                <div className="text-xs text-zinc-400">Can Retry</div>
                <div className="mt-1 font-semibold text-zinc-950">{delivery.can_retry ? "yes" : "no"}</div>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      {roles.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not load role summary" body={errorMessage(roles.error, "Role summary failed.")} />
        </div>
      ) : null}
    </Panel>
  );
}

function ProjectRoleManager({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingRole, setEditingRole] = useState<ProjectRole | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [permissions, setPermissions] = useState<ProjectPermission[]>(["project.read"]);

  const roles = useQuery({
    queryKey: queryKeys.projectRoles(projectId),
    queryFn: () => api.listProjectRoles(projectId),
  });

  const createRole = useMutation({
    mutationFn: () => api.createProjectRole(projectId, { name: name.trim(), description: description.trim(), permissions }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectRoles(projectId) });
      closeModal();
      toast.success("Role created");
    },
  });

  const updateRole = useMutation({
    mutationFn: () =>
      api.updateProjectRole(projectId, editingRole?.id || "", {
        name: name.trim(),
        description: description.trim(),
        permissions,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectRoles(projectId) });
      closeModal();
      toast.success("Role updated");
    },
  });

  const deleteRole = useMutation({
    mutationFn: (roleId: ID) => api.deleteProjectRole(projectId, roleId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectRoles(projectId) });
      closeModal();
      toast.success("Role deleted");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not delete role.")),
  });

  function openCreate() {
    setEditingRole(null);
    setName("");
    setDescription("");
    setPermissions(["project.read"]);
    setModalOpen(true);
  }

  function openEdit(role: ProjectRole) {
    setEditingRole(role);
    setName(role.name);
    setDescription(role.description);
    setPermissions(role.permissions);
    setModalOpen(true);
  }

  function closeModal() {
    setModalOpen(false);
    setEditingRole(null);
  }

  function submitRole(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (editingRole) {
      updateRole.mutate();
      return;
    }
    createRole.mutate();
  }

  const pending = createRole.isPending || updateRole.isPending;

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 className="text-base font-semibold text-zinc-950">Project Roles</h2>
          <p className="mt-1 text-sm text-zinc-500">Project-local permission sets.</p>
        </div>
        <Button tone="primary" onClick={openCreate}>
          <Plus size={16} />
          Role
        </Button>
      </div>

      {roles.isLoading ? <LoadingState label="Loading roles" /> : null}
      {roles.isError ? <ErrorState title="Could not load roles" body={errorMessage(roles.error, "Role request failed.")} /> : null}

      <div className="divide-y divide-zinc-100">
        {roles.data?.map((role) => (
          <div key={role.id} className="grid gap-3 px-4 py-3 lg:grid-cols-[1fr_1fr_auto] lg:items-center">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-zinc-950">{role.name}</span>
                {role.is_system ? (
                  <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs text-zinc-500">system</span>
                ) : null}
              </div>
              <div className="mt-1 truncate text-xs text-zinc-500">
                {role.key} / {compactId(role.id)}
              </div>
            </div>
            <div className="min-w-0 text-sm text-zinc-600">
              <div>{permissionsSummary(role)}</div>
              <div className="mt-1 line-clamp-1 text-xs text-zinc-500">{role.description || "No description"}</div>
            </div>
            <div className="flex justify-end">
              <Button onClick={() => openEdit(role)}>
                <Pencil size={15} />
                Edit
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Modal
        open={modalOpen}
        title={editingRole ? "Edit Role" : "Create Role"}
        onClose={closeModal}
        formId="project-role"
        onSubmit={submitRole}
        footer={
          <>
            {editingRole && !editingRole.is_system ? (
              <Button
                tone="danger"
                disabled={deleteRole.isPending}
                onClick={() => {
                  if (window.confirm("Delete this role?")) {
                    deleteRole.mutate(editingRole.id);
                  }
                }}
              >
                <Trash2 size={16} />
                Delete
              </Button>
            ) : null}
            <div className="flex flex-1 justify-end gap-2">
              <Button onClick={closeModal}>Cancel</Button>
              <Button type="submit" form="project-role" tone="primary" disabled={pending || !name.trim() || permissions.length === 0}>
                Save
              </Button>
            </div>
          </>
        }
      >
        <div className="grid gap-4">
          {createRole.isError ? (
            <ErrorState title="Could not create role" body={errorMessage(createRole.error, "Role creation failed.")} />
          ) : null}
          {updateRole.isError ? (
            <ErrorState title="Could not update role" body={errorMessage(updateRole.error, "Role update failed.")} />
          ) : null}
          <TextField label="Name" value={name} onChange={(event) => setName(event.target.value)} required />
          <TextAreaField label="Description" value={description} onChange={(event) => setDescription(event.target.value)} />
          <PermissionPicker value={permissions} onChange={setPermissions} />
        </div>
      </Modal>
    </Panel>
  );
}

function ProjectMemberActions({ project }: { project: Project }) {
  const queryClient = useQueryClient();
  const projectId = project.id;
  const [inviteRef, setInviteRef] = useState("");
  const [inviteRoleId, setInviteRoleId] = useState("");
  const [inviteStatus, setInviteStatus] = useState<ProjectInvite["status"] | "">("pending");

  const roles = useQuery({
    queryKey: queryKeys.projectRoles(projectId),
    queryFn: () => api.listProjectRoles(projectId),
  });
  const members = useQuery({
    queryKey: queryKeys.projectMembers(projectId),
    queryFn: () => api.listProjectMembers(projectId),
  });
  const invites = useQuery({
    queryKey: queryKeys.projectInvites(projectId, inviteStatus),
    queryFn: () => api.listProjectInvites(projectId, { status: inviteStatus }),
  });

  async function refreshMembership() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.projectMembers(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projectInvitesScope(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.myProjectInvitesScope }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projects }),
    ]);
  }

  const inviteMember = useMutation({
    mutationFn: () => api.inviteProjectMember(projectId, { user_ref: inviteRef.trim(), role_id: inviteRoleId || undefined }),
    onSuccess: async () => {
      await refreshMembership();
      setInviteRef("");
      toast.success("Invite created");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not invite member.")),
  });

  const removeMember = useMutation({
    mutationFn: (userId: ID) => api.removeProjectMember(projectId, userId),
    onSuccess: async () => {
      await Promise.all([
        refreshMembership(),
        queryClient.invalidateQueries({ queryKey: queryKeys.tickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graph(projectId) }),
      ]);
      toast.success("Member removed");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not remove member.")),
  });

  const updateMemberRole = useMutation({
    mutationFn: ({ userId, roleId }: { userId: ID; roleId: ID }) => api.updateProjectMemberRole(projectId, userId, { role_id: roleId }),
    onSuccess: async () => {
      await refreshMembership();
      toast.success("Role updated");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not update member role.")),
  });

  const revokeInvite = useMutation({
    mutationFn: (inviteId: ID) => api.revokeInvite(inviteId),
    onSuccess: async () => {
      await refreshMembership();
      toast.success("Invite revoked");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not revoke invite.")),
  });

  const memberRows = members.data || [];
  const inviteRows = invites.data || [];
  const availableRoles = roles.data || [];

  function copyID(id: ID) {
    void navigator.clipboard?.writeText(id);
    toast.success("ID copied");
  }

  function exportMembersCSV() {
    downloadText(`${safeFilePart(project.name)}-members.csv`, "text/csv", `${membersToCSV(memberRows)}\n`);
  }

  function exportInvitesJSON() {
    const report = {
      project: { id: project.id, name: project.name, handle: project.handle },
      generated_at: new Date().toISOString(),
      status: inviteStatus || "all",
      members: memberRows,
      invites: inviteRows,
    };
    downloadText(`${safeFilePart(project.name)}-membership.json`, "application/json", `${JSON.stringify(report, null, 2)}\n`);
  }

  function exportInvitesCSV() {
    downloadText(`${safeFilePart(project.name)}-invites.csv`, "text/csv", `${invitesToCSV(inviteRows)}\n`);
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
            <Users size={17} />
            Membership
          </h2>
          <p className="mt-1 text-sm text-zinc-500">Manage project people, pending invitations, and membership exports.</p>
        </div>
        <div className="grid grid-cols-2 gap-2 text-sm sm:flex">
          <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2">
            <div className="text-xs text-zinc-400">Members</div>
            <div className="mt-0.5 font-semibold text-zinc-950">{members.isLoading ? "..." : memberRows.length}</div>
          </div>
          <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2">
            <div className="text-xs text-zinc-400">Invites</div>
            <div className="mt-0.5 font-semibold text-zinc-950">{invites.isLoading ? "..." : inviteRows.length}</div>
          </div>
        </div>
      </div>

      <div className="border-t border-zinc-100 p-4">
        <form
          className="grid gap-3 lg:grid-cols-[1fr_240px_auto]"
          onSubmit={(event) => {
            event.preventDefault();
            inviteMember.mutate();
          }}
        >
          <TextField
            label="Invitee"
            placeholder="alice@example.com"
            value={inviteRef}
            onChange={(event) => setInviteRef(event.target.value)}
            required
          />
          <SelectField label="Role" value={inviteRoleId} onChange={(event) => setInviteRoleId(event.target.value)}>
            <option value="">Default role</option>
            {roles.data?.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name}
              </option>
            ))}
          </SelectField>
          <Button className="self-end" type="submit" tone="primary" disabled={inviteMember.isPending || !inviteRef.trim()}>
            <Send size={16} />
            Invite
          </Button>
        </form>
      </div>

      {roles.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not load roles" body={errorMessage(roles.error, "Role request failed.")} />
        </div>
      ) : null}

      <div className="grid gap-4 border-t border-zinc-100 p-4 xl:grid-cols-[1fr_1fr]">
        <section className="min-w-0 rounded-2xl border border-zinc-200">
          <div className="flex flex-col gap-3 border-b border-zinc-100 p-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
                <UserPlus size={15} />
                Members
              </h3>
              <p className="mt-1 text-xs text-zinc-500">Accepted local users on this project.</p>
            </div>
            <Button onClick={exportMembersCSV} disabled={memberRows.length === 0}>
              <Download size={15} />
              CSV
            </Button>
          </div>
          {members.isLoading ? <LoadingState label="Loading members" /> : null}
          {members.isError ? (
            <div className="p-3">
              <ErrorState title="Could not load members" body={errorMessage(members.error, "Member request failed.")} />
            </div>
          ) : null}
          {!members.isLoading && !members.isError && memberRows.length === 0 ? (
            <div className="p-6 text-center text-sm text-zinc-400">No members yet.</div>
          ) : null}
          <div className="divide-y divide-zinc-100">
            {memberRows.map((member) => {
              const title = actorTitle(member.name, member.username, member.handle);
              return (
                <div key={member.user_id} className="grid gap-3 p-3 md:grid-cols-[1fr_auto] md:items-center">
                  <div className="flex min-w-0 items-center gap-3">
                    <PersonMark label={title} />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-medium text-zinc-950">{title}</span>
                        {member.user_id === project.owner_id ? (
                          <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs text-zinc-500">owner</span>
                        ) : null}
                      </div>
                      <div className="mt-1 truncate text-xs text-zinc-500">{actorSubtitle(member.handle, member.email)}</div>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <label className="grid gap-1 text-xs text-zinc-500">
                      <span className="sr-only">Role for {title}</span>
                      <select
                        className="focus-ring h-9 min-w-40 rounded-full border border-zinc-200 bg-white px-3 text-sm font-medium text-zinc-800 shadow-sm disabled:opacity-50"
                        value={member.role_id}
                        disabled={roles.isLoading || roles.isError || updateMemberRole.isPending}
                        onChange={(event) => {
                          const roleId = event.target.value;
                          if (roleId && roleId !== member.role_id) {
                            updateMemberRole.mutate({ userId: member.user_id, roleId });
                          }
                        }}
                      >
                        {availableRoles.some((role) => role.id === member.role_id) ? null : (
                          <option value={member.role_id}>{member.role_name || member.role}</option>
                        )}
                        {availableRoles.map((role) => (
                          <option key={role.id} value={role.id}>
                            {role.name}
                          </option>
                        ))}
                      </select>
                    </label>
                    <Button
                      tone="danger"
                      disabled={removeMember.isPending || updateMemberRole.isPending}
                      onClick={() => {
                        if (window.confirm(`Remove ${title} from this project?`)) {
                          removeMember.mutate(member.user_id);
                        }
                      }}
                    >
                      <UserMinus size={15} />
                      Remove
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </section>

        <section className="min-w-0 rounded-2xl border border-zinc-200">
          <div className="flex flex-col gap-3 border-b border-zinc-100 p-3">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
                  <History size={15} />
                  Invites
                </h3>
                <p className="mt-1 text-xs text-zinc-500">Pending and historical invite activity.</p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button onClick={exportInvitesCSV} disabled={inviteRows.length === 0}>
                  <Download size={15} />
                  CSV
                </Button>
                <Button onClick={exportInvitesJSON}>
                  <FileJson size={15} />
                  JSON
                </Button>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {(["pending", "accepted", "rejected", "revoked", ""] as Array<ProjectInvite["status"] | "">).map((status) => (
                <button
                  key={status || "all"}
                  type="button"
                  className={`focus-ring rounded-full border px-3 py-1 text-xs font-medium ${
                    inviteStatus === status
                      ? "border-zinc-950 bg-zinc-950 text-white"
                      : "border-zinc-200 bg-white text-zinc-600 hover:bg-zinc-50"
                  }`}
                  onClick={() => setInviteStatus(status)}
                >
                  {status || "all"}
                </button>
              ))}
            </div>
          </div>
          {invites.isLoading ? <LoadingState label="Loading invites" /> : null}
          {invites.isError ? (
            <div className="p-3">
              <ErrorState title="Could not load invites" body={errorMessage(invites.error, "Invite request failed.")} />
            </div>
          ) : null}
          {!invites.isLoading && !invites.isError && inviteRows.length === 0 ? (
            <div className="p-6 text-center text-sm text-zinc-400">No invites for this filter.</div>
          ) : null}
          <div className="divide-y divide-zinc-100">
            {inviteRows.map((invite) => {
              const title = actorTitle(invite.invitee_name, invite.invitee_username, invite.invitee_handle);
              return (
                <div key={invite.id} className="grid gap-3 p-3 md:grid-cols-[1fr_auto] md:items-center">
                  <div className="flex min-w-0 items-center gap-3">
                    <PersonMark label={title} />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-medium text-zinc-950">{title}</span>
                        <InviteStatusBadge status={invite.status} />
                      </div>
                      <div className="mt-1 truncate text-xs text-zinc-500">{actorSubtitle(invite.invitee_handle, invite.invitee_email)}</div>
                      <div className="mt-1 flex items-center gap-1 text-xs text-zinc-400">
                        <Mail size={12} />
                        invited by {invite.inviter_username} / {relativeDate(invite.created_at)}
                      </div>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <span className="rounded-full border border-zinc-200 bg-white px-2 py-0.5 text-xs font-medium text-zinc-700">
                      {invite.role_name || invite.role}
                    </span>
                    <Button onClick={() => copyID(invite.id)}>
                      <Copy size={15} />
                      ID
                    </Button>
                    {invite.status === "pending" ? (
                      <Button tone="danger" disabled={revokeInvite.isPending} onClick={() => revokeInvite.mutate(invite.id)}>
                        <Trash2 size={15} />
                        Revoke
                      </Button>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      </div>
    </Panel>
  );
}

function GitHubRepositoryManager({ projectId }: { projectId: ID }) {
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
    const report = {
      generated_at: new Date().toISOString(),
      repositories: repoRows,
      commits: commitRows,
    };
    downloadText(`github-integration-${projectId}.json`, "application/json", `${JSON.stringify(report, null, 2)}\n`);
  }

  function exportGitHubCSV() {
    downloadText(`github-repositories-${projectId}.csv`, "text/csv", `${githubRepositoriesToCSV(repoRows)}\n`);
  }

  function exportCommitsCSV() {
    downloadText(`github-commits-${projectId}.csv`, "text/csv", `${githubCommitsToCSV(commitRows)}\n`);
  }

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
            <Github size={17} />
            GitHub Repositories
          </h2>
          <p className="mt-1 text-sm text-zinc-500">Project repositories, imported commits, ticket references, and sync health.</p>
        </div>
        <form className="flex flex-col gap-2 sm:flex-row" onSubmit={submitRepository}>
          <TextField
            label="Repository"
            className="min-w-64"
            placeholder="owner/repository"
            value={repoRef}
            onChange={(event) => setRepoRef(event.target.value)}
          />
          <Button type="submit" tone="primary" className="self-end" disabled={linkRepository.isPending || !repoRef.trim()}>
            <Plus size={16} />
            Link
          </Button>
        </form>
      </div>

      <div className="grid gap-3 border-t border-zinc-100 p-4 sm:grid-cols-2 xl:grid-cols-5">
        <MetricPill icon={<Github size={14} />} label="Repos" value={repositories.isLoading ? "..." : repoRows.length} />
        <MetricPill icon={<History size={14} />} label="Commits" value={repositories.isLoading ? "..." : totalCommits} />
        <MetricPill icon={<Link2 size={14} />} label="Linked" value={repositories.isLoading ? "..." : linkedCommits} />
        <MetricPill icon={<Pencil size={14} />} label="Manual" value={repositories.isLoading ? "..." : manualLinks} />
        <MetricPill icon={<Clock3 size={14} />} label="Webhook" value={latestWebhook ? relativeDate(latestWebhook) : "none"} />
      </div>

      {formError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not link repository" body={formError} />
        </div>
      ) : null}

      <div className="border-t border-zinc-100 p-4">
        {repositories.isLoading ? <LoadingState label="Loading GitHub repositories" /> : null}
        {repositories.isError ? (
          <ErrorState title="Could not load GitHub repositories" body={errorMessage(repositories.error, "Repository request failed.")} />
        ) : null}
        {!repositories.isLoading && !repositories.isError && repoRows.length === 0 ? (
          <div className="rounded-xl border border-dashed border-zinc-300 px-4 py-6 text-sm text-zinc-500">No GitHub repositories linked.</div>
        ) : null}
        <div className="grid gap-2">
          {repoRows.map((repo: GitHubRepository) => {
            const syncing = syncRepository.isPending && syncRepository.variables === repo.id;
            const removing = deleteRepository.isPending && deleteRepository.variables === repo.id;
            return (
              <div key={repo.id} className="rounded-xl border border-zinc-200 px-3 py-3">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0">
                    <a
                      className="inline-flex max-w-full items-center gap-2 truncate text-sm font-semibold text-zinc-950 underline-offset-4 hover:underline"
                      href={repo.html_url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      <Github size={15} className="shrink-0" />
                      <span className="truncate">{repo.full_name}</span>
                    </a>
                    <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-zinc-500">
                      <span className="inline-flex items-center gap-1">
                        <GitBranch size={13} />
                        {repo.default_branch || "default"}
                      </span>
                      <span>{repo.last_synced_at ? `Synced ${relativeDate(repo.last_synced_at)}` : "Never synced"}</span>
                      <span>{repo.last_webhook_at ? `Webhook ${relativeDate(repo.last_webhook_at)}` : "No webhook"}</span>
                      <span>{repo.commit_count} commits</span>
                      <span>{repo.linked_commit_count} linked</span>
                    </div>
                    {repo.last_sync_error ? (
                      <div className="mt-2 flex items-center gap-1 text-xs text-red-600">
                        <AlertTriangle size={13} />
                        <span className="line-clamp-1">{repo.last_sync_error}</span>
                      </div>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button onClick={() => syncRepository.mutate(repo.id)} disabled={syncing || removing}>
                      <RefreshCw size={16} />
                      {syncing ? "Syncing" : "Sync"}
                    </Button>
                    <Button
                      tone="danger"
                      onClick={() => {
                        if (window.confirm(`Remove ${repo.full_name}?`)) {
                          deleteRepository.mutate(repo.id);
                        }
                      }}
                      disabled={syncing || removing}
                    >
                      <Trash2 size={16} />
                      Remove
                    </Button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="border-t border-zinc-100 p-4">
        <div className="mb-3 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h3 className="text-sm font-semibold text-zinc-950">Recent Commits</h3>
            <p className="mt-1 text-xs text-zinc-500">Imported commit activity across linked repositories.</p>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <TextField
              label="Search"
              className="min-w-64"
              placeholder="message, SHA, author"
              value={commitSearch}
              onChange={(event) => setCommitSearch(event.target.value)}
            />
            <Button className="self-end" onClick={exportGitHubCSV} disabled={repoRows.length === 0}>
              <Download size={15} />
              Repos CSV
            </Button>
            <Button className="self-end" onClick={exportCommitsCSV} disabled={commitRows.length === 0}>
              <Download size={15} />
              Commits CSV
            </Button>
            <Button className="self-end" onClick={exportGitHubJSON}>
              <FileJson size={15} />
              JSON
            </Button>
          </div>
        </div>

        {commits.isLoading ? <LoadingState label="Loading GitHub commits" /> : null}
        {commits.isError ? (
          <ErrorState title="Could not load GitHub commits" body={errorMessage(commits.error, "Commit request failed.")} />
        ) : null}
        {!commits.isLoading && !commits.isError && commitRows.length === 0 ? (
          <div className="rounded-xl border border-dashed border-zinc-300 px-4 py-6 text-sm text-zinc-500">No imported commits found.</div>
        ) : null}
        <div className="divide-y divide-zinc-100 rounded-xl border border-zinc-200">
          {commitRows.map((commit) => (
            <div key={commit.id} className="grid gap-3 p-3 lg:grid-cols-[1fr_auto] lg:items-center">
              <div className="min-w-0">
                <a
                  className="line-clamp-1 text-sm font-medium text-zinc-950 underline-offset-4 hover:underline"
                  href={commit.html_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  {commitTitle(commit.message)}
                </a>
                <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-zinc-500">
                  <span>{commit.repository_full_name}</span>
                  <span>{commit.author_name || "Unknown author"}</span>
                  <span>{commit.authored_at ? relativeDate(commit.authored_at) : "unknown date"}</span>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2 lg:justify-end">
                <code className="rounded-md bg-zinc-50 px-1.5 py-0.5 text-xs text-zinc-600">{commit.short_sha}</code>
                <span className="rounded-full border border-zinc-200 bg-white px-2 py-0.5 text-xs text-zinc-600">
                  {commit.ticket_ids.length} tickets
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </Panel>
  );
}

export function ProjectSettingsPanel({ project, tickets }: { project: Project; tickets: Ticket[] }) {
  return (
    <div className="space-y-4">
      <ProjectAdminOverview project={project} tickets={tickets} />

      <Panel className="p-4">
        <div className="grid gap-3 lg:grid-cols-3">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">Handle</div>
            <div className="mt-1 break-all text-sm text-zinc-950">{project.handle}</div>
          </div>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">Owner</div>
            <div className="mt-1 break-all text-sm text-zinc-950">{project.owner_id}</div>
          </div>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">Updated</div>
            <div className="mt-1 text-sm text-zinc-950">{relativeDate(project.updated_at)}</div>
          </div>
        </div>
      </Panel>

      <GitHubRepositoryManager projectId={project.id} />
      <ProjectRoleManager projectId={project.id} />
      <ProjectMemberActions project={project} />
    </div>
  );
}

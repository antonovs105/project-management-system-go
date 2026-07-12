import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Download, FileJson, History, Mail, Pencil, Plus, Send, Trash2, UserMinus, UserPlus, Users } from "lucide-react";
import { lazy, Suspense, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Modal, Panel, SelectField, TextAreaField, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { projectPermissionGroups } from "../../lib/constants";
import { compactId, initials, relativeDate } from "../../lib/format";
import { fieldLimits } from "../../lib/limits";
import { queryKeys } from "../../lib/queryKeys";
import type {
  ID,
  Project,
  ProjectInvite,
  ProjectPermission,
  ProjectRole,
  Ticket,
} from "../../types";
import { ProjectLabelsPanel } from "./ProjectLabelsPanel";
import { ProjectAdminOverview } from "./ProjectAdminOverview";
import { ProjectWebhooksPanel } from "./ProjectWebhooksPanel";
import { downloadText, invitesToCSV, membersToCSV, safeFilePart } from "./projectSettingsExports";

const ProjectGitHubPanel = lazy(() => import("./ProjectGitHubPanel").then((module) => ({ default: module.ProjectGitHubPanel })));

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

function actorTitle(name: string, username: string, handle: string): string {
  return name || username || handle || "Unknown actor";
}

function actorSubtitle(handle: string, email?: string): string {
  if (email) {
    return `${handle} / ${email}`;
  }
  return handle || "No handle";
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
          <TextField
            label="Name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={fieldLimits.projectRoleNameMaxLength}
            required
          />
          <TextAreaField
            label="Description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            maxLength={fieldLimits.projectRoleDescriptionMaxLength}
          />
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
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) }),
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
              <p className="mt-1 text-xs text-zinc-500">Accepted local and remote collaborators on this project.</p>
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
                        {member.is_remote ? (
                          <span className="rounded-full border border-zinc-200 bg-zinc-950 px-2 py-0.5 text-xs text-white">remote</span>
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

      <Suspense fallback={<LoadingState label="Loading GitHub integration" />}>
        <ProjectGitHubPanel projectId={project.id} />
      </Suspense>
      <ProjectWebhooksPanel projectId={project.id} />
      <ProjectLabelsPanel projectId={project.id} />
      <ProjectRoleManager projectId={project.id} />
      <ProjectMemberActions project={project} />
    </div>
  );
}

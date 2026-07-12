import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Download, FileJson, History, Mail, Pencil, Plus, Send, Trash2, UserMinus, UserPlus, Users } from "lucide-react";
import { lazy, Suspense, useMemo, useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Modal, Panel, SelectField, TextAreaField, TextField } from "../../components/ui";
import { OffsetPaginationControls } from "../../components/OffsetPaginationControls";
import { api, errorMessage } from "../../lib/api";
import { projectPermissionGroups } from "../../lib/constants";
import { compactId, initials } from "../../lib/format";
import { fieldLimits } from "../../lib/limits";
import { queryKeys } from "../../lib/queryKeys";
import { useI18n } from "../../lib/i18n-context";
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

function permissionsSummary(role: ProjectRole, all: string, none: string, count: string): string {
  if (role.permissions.length === allProjectPermissions().length) {
    return all;
  }
  if (role.permissions.length === 0) {
    return none;
  }
  return count;
}

function actorTitle(name: string, username: string, handle: string, fallback: string): string {
  return name || username || handle || fallback;
}

function actorSubtitle(handle: string, email: string | undefined, fallback: string): string {
  if (email) {
    return `${handle} / ${email}`;
  }
  return handle || fallback;
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
  const { t } = useI18n();
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
      toast.success(t("settings.roleCreated"));
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
      toast.success(t("settings.roleUpdated"));
    },
  });

  const deleteRole = useMutation({
    mutationFn: (roleId: ID) => api.deleteProjectRole(projectId, roleId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projectRoles(projectId) });
      closeModal();
      toast.success(t("settings.roleDeleted"));
    },
    onError: (error) => toast.error(errorMessage(error, t("settings.roleDeleteFailed"))),
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
          <h2 className="text-base font-semibold text-zinc-950">{t("settings.rolesTitle")}</h2>
          <p className="mt-1 text-sm text-zinc-500">{t("settings.rolesBody")}</p>
        </div>
        <Button tone="primary" onClick={openCreate}>
          <Plus size={16} />
          {t("settings.role")}
        </Button>
      </div>

      {roles.isLoading ? <LoadingState label={t("settings.rolesLoading")} /> : null}
      {roles.isError ? <ErrorState title={t("settings.rolesLoadFailed")} body={errorMessage(roles.error, t("settings.rolesRequestFailed"))} /> : null}

      <div className="divide-y divide-zinc-100">
        {roles.data?.map((role) => (
          <div key={role.id} className="grid gap-3 px-4 py-3 lg:grid-cols-[1fr_1fr_auto] lg:items-center">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-zinc-950">{role.name}</span>
                {role.is_system ? (
                  <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs text-zinc-500">{t("settings.system")}</span>
                ) : null}
              </div>
              <div className="mt-1 truncate text-xs text-zinc-500">
                {role.key} / {compactId(role.id)}
              </div>
            </div>
            <div className="min-w-0 text-sm text-zinc-600">
              <div>{permissionsSummary(role, t("settings.allPermissions"), t("settings.noPermissions"), t("settings.permissionCount", { count: role.permissions.length }))}</div>
              <div className="mt-1 line-clamp-1 text-xs text-zinc-500">{role.description || t("common.noDescription")}</div>
            </div>
            <div className="flex justify-end">
              <Button onClick={() => openEdit(role)}>
                <Pencil size={15} />
                {t("actions.edit")}
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Modal
        open={modalOpen}
        title={editingRole ? t("settings.editRole") : t("settings.createRole")}
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
                  if (window.confirm(t("settings.deleteRoleConfirm"))) {
                    deleteRole.mutate(editingRole.id);
                  }
                }}
              >
                <Trash2 size={16} />
                {t("actions.delete")}
              </Button>
            ) : null}
            <div className="flex flex-1 justify-end gap-2">
              <Button onClick={closeModal}>{t("actions.cancel")}</Button>
              <Button type="submit" form="project-role" tone="primary" disabled={pending || !name.trim() || permissions.length === 0}>
                {t("actions.save")}
              </Button>
            </div>
          </>
        }
      >
        <div className="grid gap-4">
          {createRole.isError ? (
            <ErrorState title={t("settings.roleCreateFailed")} body={errorMessage(createRole.error, t("settings.roleCreationFailed"))} />
          ) : null}
          {updateRole.isError ? (
            <ErrorState title={t("settings.roleUpdateFailed")} body={errorMessage(updateRole.error, t("settings.roleUpdateRequestFailed"))} />
          ) : null}
          <TextField
            label={t("projects.name")}
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={fieldLimits.projectRoleNameMaxLength}
            required
          />
          <TextAreaField
            label={t("projects.description")}
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
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const projectId = project.id;
  const [inviteRef, setInviteRef] = useState("");
  const [inviteRoleId, setInviteRoleId] = useState("");
  const [inviteStatus, setInviteStatus] = useState<ProjectInvite["status"] | "">("pending");
  const [memberOffset, setMemberOffset] = useState(0);
  const [inviteOffset, setInviteOffset] = useState(0);

  const roles = useQuery({
    queryKey: queryKeys.projectRoles(projectId),
    queryFn: () => api.listProjectRoles(projectId),
  });
  const members = useQuery({
    queryKey: queryKeys.projectMembersPage(projectId, memberOffset),
    queryFn: () => api.listProjectMembersPage(projectId, { limit: 25, offset: memberOffset }),
  });
  const invites = useQuery({
    queryKey: queryKeys.projectInvitesPage(projectId, inviteStatus, inviteOffset),
    queryFn: () => api.listProjectInvitesPage(projectId, { limit: 25, offset: inviteOffset }, { status: inviteStatus }),
  });

  async function refreshMembership() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.projectMembers(projectId) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projectMembersPageScope(projectId) }),
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
      toast.success(t("settings.inviteCreated"));
    },
    onError: (error) => toast.error(errorMessage(error, t("settings.inviteFailed"))),
  });

  const removeMember = useMutation({
    mutationFn: (userId: ID) => api.removeProjectMember(projectId, userId),
    onSuccess: async () => {
      await Promise.all([
        refreshMembership(),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.graphScope(projectId) }),
      ]);
      toast.success(t("settings.memberRemoved"));
    },
    onError: (error) => toast.error(errorMessage(error, t("settings.memberRemoveFailed"))),
  });

  const updateMemberRole = useMutation({
    mutationFn: ({ userId, roleId }: { userId: ID; roleId: ID }) => api.updateProjectMemberRole(projectId, userId, { role_id: roleId }),
    onSuccess: async () => {
      await refreshMembership();
      toast.success(t("settings.roleUpdated"));
    },
    onError: (error) => toast.error(errorMessage(error, t("settings.memberRoleFailed"))),
  });

  const revokeInvite = useMutation({
    mutationFn: (inviteId: ID) => api.revokeInvite(inviteId),
    onSuccess: async () => {
      await refreshMembership();
      toast.success(t("settings.inviteRevoked"));
    },
    onError: (error) => toast.error(errorMessage(error, t("settings.inviteRevokeFailed"))),
  });

  const memberRows = members.data?.items || [];
  const inviteRows = invites.data?.items || [];
  const availableRoles = roles.data || [];

  function copyID(id: ID) {
    void navigator.clipboard?.writeText(id);
    toast.success(t("settings.idCopied"));
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
            {t("settings.membershipTitle")}
          </h2>
          <p className="mt-1 text-sm text-zinc-500">{t("settings.peopleBody")}</p>
        </div>
        <div className="grid grid-cols-2 gap-2 text-sm sm:flex">
          <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2">
            <div className="text-xs text-zinc-400">{t("settings.members")}</div>
            <div className="mt-0.5 font-semibold text-zinc-950">{members.isLoading ? "..." : memberRows.length}</div>
          </div>
          <div className="rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2">
            <div className="text-xs text-zinc-400">{t("settings.invites")}</div>
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
            label={t("settings.invitee")}
            placeholder="alice@example.com"
            value={inviteRef}
            onChange={(event) => setInviteRef(event.target.value)}
            required
          />
          <SelectField label={t("settings.role")} value={inviteRoleId} onChange={(event) => setInviteRoleId(event.target.value)}>
            <option value="">{t("settings.defaultRole")}</option>
            {roles.data?.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name}
              </option>
            ))}
          </SelectField>
          <Button className="self-end" type="submit" tone="primary" disabled={inviteMember.isPending || !inviteRef.trim()}>
            <Send size={16} />
            {t("settings.invite")}
          </Button>
        </form>
      </div>

      {roles.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("settings.rolesLoadFailed")} body={errorMessage(roles.error, t("settings.rolesRequestFailed"))} />
        </div>
      ) : null}

      <div className="grid gap-4 border-t border-zinc-100 p-4 xl:grid-cols-[1fr_1fr]">
        <section className="min-w-0 rounded-2xl border border-zinc-200">
          <div className="flex flex-col gap-3 border-b border-zinc-100 p-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
                <UserPlus size={15} />
                {t("settings.members")}
              </h3>
              <p className="mt-1 text-xs text-zinc-500">{t("settings.membersBody")}</p>
            </div>
            <Button onClick={exportMembersCSV} disabled={memberRows.length === 0}>
              <Download size={15} />
              CSV
            </Button>
          </div>
          {members.isLoading ? <LoadingState label={t("settings.membersLoading")} /> : null}
          {members.isError ? (
            <div className="p-3">
              <ErrorState title={t("settings.membersLoadFailed")} body={errorMessage(members.error, t("settings.memberRequestFailed"))} />
            </div>
          ) : null}
          {!members.isLoading && !members.isError && memberRows.length === 0 ? (
            <div className="p-6 text-center text-sm text-zinc-400">{t("settings.noMembers")}</div>
          ) : null}
          <div className="divide-y divide-zinc-100">
            {memberRows.map((member) => {
              const title = actorTitle(member.name, member.username, member.handle, t("settings.unknownActor"));
              return (
                <div key={member.user_id} className="grid gap-3 p-3 md:grid-cols-[1fr_auto] md:items-center">
                  <div className="flex min-w-0 items-center gap-3">
                    <PersonMark label={title} />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-medium text-zinc-950">{title}</span>
                        {member.user_id === project.owner_id ? (
                          <span className="rounded-full border border-zinc-200 bg-zinc-50 px-2 py-0.5 text-xs text-zinc-500">{t("settings.owner")}</span>
                        ) : null}
                        {member.is_remote ? (
                          <span className="rounded-full border border-zinc-200 bg-zinc-950 px-2 py-0.5 text-xs text-white">{t("settings.remote")}</span>
                        ) : null}
                      </div>
                      <div className="mt-1 truncate text-xs text-zinc-500">{actorSubtitle(member.handle, member.email, t("settings.noHandle"))}</div>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <label className="grid gap-1 text-xs text-zinc-500">
                      <span className="sr-only">{t("settings.roleFor", { name: title })}</span>
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
                        if (window.confirm(t("settings.removeMemberConfirm", { name: title }))) {
                          removeMember.mutate(member.user_id);
                        }
                      }}
                    >
                      <UserMinus size={15} />
                      {t("settings.remove")}
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
          {members.data ? (
            <div className="border-t border-zinc-100 p-3">
              <OffsetPaginationControls page={members.data} onOffsetChange={setMemberOffset} disabled={members.isFetching} />
            </div>
          ) : null}
        </section>

        <section className="min-w-0 rounded-2xl border border-zinc-200">
          <div className="flex flex-col gap-3 border-b border-zinc-100 p-3">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h3 className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
                  <History size={15} />
                  {t("settings.invites")}
                </h3>
                <p className="mt-1 text-xs text-zinc-500">{t("settings.invitesBody")}</p>
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
                  onClick={() => {
                    setInviteStatus(status);
                    setInviteOffset(0);
                  }}
                >
                  {status ? t(`status.${status}`) : t("common.all")}
                </button>
              ))}
            </div>
          </div>
          {invites.isLoading ? <LoadingState label={t("settings.invitesLoading")} /> : null}
          {invites.isError ? (
            <div className="p-3">
              <ErrorState title={t("settings.invitesLoadFailed")} body={errorMessage(invites.error, t("settings.inviteRequestFailed"))} />
            </div>
          ) : null}
          {!invites.isLoading && !invites.isError && inviteRows.length === 0 ? (
            <div className="p-6 text-center text-sm text-zinc-400">{t("settings.noInvites")}</div>
          ) : null}
          <div className="divide-y divide-zinc-100">
            {inviteRows.map((invite) => {
              const title = actorTitle(invite.invitee_name, invite.invitee_username, invite.invitee_handle, t("settings.unknownActor"));
              return (
                <div key={invite.id} className="grid gap-3 p-3 md:grid-cols-[1fr_auto] md:items-center">
                  <div className="flex min-w-0 items-center gap-3">
                    <PersonMark label={title} />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="truncate font-medium text-zinc-950">{title}</span>
                        <InviteStatusBadge status={invite.status} />
                      </div>
                      <div className="mt-1 truncate text-xs text-zinc-500">{actorSubtitle(invite.invitee_handle, invite.invitee_email, t("settings.noHandle"))}</div>
                      <div className="mt-1 flex items-center gap-1 text-xs text-zinc-400">
                        <Mail size={12} />
                        {t("settings.invitedBy", { name: invite.inviter_username, date: relativeDate(invite.created_at) })}
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
                        {t("settings.revoke")}
                      </Button>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
          {invites.data ? (
            <div className="border-t border-zinc-100 p-3">
              <OffsetPaginationControls page={invites.data} onOffsetChange={setInviteOffset} disabled={invites.isFetching} />
            </div>
          ) : null}
        </section>
      </div>
    </Panel>
  );
}

export function ProjectSettingsPanel({ project, tickets }: { project: Project; tickets: Ticket[] }) {
  const { t, relativeDate } = useI18n();
  return (
    <div className="space-y-4">
      <ProjectAdminOverview project={project} tickets={tickets} />

      <Panel className="p-4">
        <div className="grid gap-3 lg:grid-cols-3">
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("settings.handle")}</div>
            <div className="mt-1 break-all text-sm text-zinc-950">{project.handle}</div>
          </div>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("settings.ownerLabel")}</div>
            <div className="mt-1 break-all text-sm text-zinc-950">{project.owner_id}</div>
          </div>
          <div>
            <div className="text-xs font-semibold uppercase tracking-wide text-zinc-400">{t("settings.updated")}</div>
            <div className="mt-1 text-sm text-zinc-950">{relativeDate(project.updated_at)}</div>
          </div>
        </div>
      </Panel>

      <Suspense fallback={<LoadingState label={t("settings.loadingGitHub")} />}>
        <ProjectGitHubPanel projectId={project.id} />
      </Suspense>
      <ProjectWebhooksPanel projectId={project.id} />
      <ProjectLabelsPanel projectId={project.id} />
      <ProjectRoleManager projectId={project.id} />
      <ProjectMemberActions project={project} />
    </div>
  );
}

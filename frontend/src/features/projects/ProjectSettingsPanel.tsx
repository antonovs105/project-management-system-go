import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Pencil, Plus, Shield, Trash2, UserMinus, UserPlus } from "lucide-react";
import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, LoadingState, Modal, Panel, SelectField, TextAreaField, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { projectPermissionGroups } from "../../lib/constants";
import { compactId, relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type { ID, Project, ProjectInvite, ProjectPermission, ProjectRole } from "../../types";

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
                {role.key} · {compactId(role.id)}
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

function ProjectMemberActions({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [inviteUserId, setInviteUserId] = useState("");
  const [inviteRoleId, setInviteRoleId] = useState("");
  const [removeUserId, setRemoveUserId] = useState("");
  const [inviteId, setInviteId] = useState("");
  const [lastInvite, setLastInvite] = useState<ProjectInvite | null>(null);

  const roles = useQuery({
    queryKey: queryKeys.projectRoles(projectId),
    queryFn: () => api.listProjectRoles(projectId),
  });

  const inviteMember = useMutation({
    mutationFn: () => api.inviteProjectMember(projectId, { user_id: inviteUserId.trim(), role_id: inviteRoleId || undefined }),
    onSuccess: async (invite) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projects });
      setLastInvite(invite);
      setInviteUserId("");
      toast.success("Invite created");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not invite member.")),
  });

  const removeMember = useMutation({
    mutationFn: () => api.removeProjectMember(projectId, removeUserId.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projects });
      setRemoveUserId("");
      toast.success("Member removed");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not remove member.")),
  });

  const updateInvite = useMutation({
    mutationFn: async (action: "accept" | "reject" | "revoke") => {
      const id = inviteId.trim();
      if (action === "accept") {
        return api.acceptInvite(id);
      }
      if (action === "reject") {
        return api.rejectInvite(id);
      }
      return api.revokeInvite(id);
    },
    onSuccess: async (invite) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.projects });
      setLastInvite(invite);
      toast.success("Invite updated");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not update invite.")),
  });

  function copyInviteId() {
    if (!lastInvite) {
      return;
    }
    void navigator.clipboard?.writeText(lastInvite.id);
    toast.success("Invite ID copied");
  }

  return (
    <Panel className="overflow-hidden">
      <div className="px-4 py-4">
        <h2 className="text-base font-semibold text-zinc-950">Membership</h2>
        <p className="mt-1 text-sm text-zinc-500">Project member and invite operations.</p>
      </div>

      <div className="grid gap-4 border-t border-zinc-100 p-4 xl:grid-cols-3">
        <form
          className="grid gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            inviteMember.mutate();
          }}
        >
          <div className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
            <UserPlus size={16} />
            Invite
          </div>
          <TextField label="User ID" value={inviteUserId} onChange={(event) => setInviteUserId(event.target.value)} required />
          <SelectField label="Role" value={inviteRoleId} onChange={(event) => setInviteRoleId(event.target.value)}>
            <option value="">Default role</option>
            {roles.data?.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name}
              </option>
            ))}
          </SelectField>
          <Button type="submit" tone="primary" disabled={inviteMember.isPending || !inviteUserId.trim()}>
            Invite
          </Button>
        </form>

        <form
          className="grid gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            removeMember.mutate();
          }}
        >
          <div className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
            <UserMinus size={16} />
            Remove
          </div>
          <TextField label="User ID" value={removeUserId} onChange={(event) => setRemoveUserId(event.target.value)} required />
          <Button type="submit" tone="danger" disabled={removeMember.isPending || !removeUserId.trim()}>
            Remove member
          </Button>
        </form>

        <div className="grid gap-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-zinc-950">
            <Shield size={16} />
            Invite Status
          </div>
          <TextField label="Invite ID" value={inviteId} onChange={(event) => setInviteId(event.target.value)} />
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => updateInvite.mutate("accept")} disabled={updateInvite.isPending || !inviteId.trim()}>
              Accept
            </Button>
            <Button onClick={() => updateInvite.mutate("reject")} disabled={updateInvite.isPending || !inviteId.trim()}>
              Reject
            </Button>
            <Button tone="danger" onClick={() => updateInvite.mutate("revoke")} disabled={updateInvite.isPending || !inviteId.trim()}>
              Revoke
            </Button>
          </div>
        </div>
      </div>

      {roles.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not load roles" body={errorMessage(roles.error, "Role request failed.")} />
        </div>
      ) : null}
      {lastInvite ? (
        <div className="border-t border-zinc-100 px-4 py-3">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="min-w-0 text-sm">
              <div className="font-medium text-zinc-950">{lastInvite.status}</div>
              <div className="mt-1 truncate text-xs text-zinc-500">
                {lastInvite.id} · {lastInvite.role}
              </div>
            </div>
            <Button onClick={copyInviteId}>
              <Copy size={15} />
              Copy ID
            </Button>
          </div>
        </div>
      ) : null}
    </Panel>
  );
}

export function ProjectSettingsPanel({ project }: { project: Project }) {
  return (
    <div className="space-y-4">
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

      <ProjectRoleManager projectId={project.id} />
      <ProjectMemberActions projectId={project.id} />
    </div>
  );
}

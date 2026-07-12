import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Search, Shield, Users } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel, SelectField, TextField } from "../components/ui";
import { OffsetPaginationControls } from "../components/OffsetPaginationControls";
import { api, errorMessage } from "../lib/api";
import { instanceRoles } from "../lib/constants";
import { compactId } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { queryKeys } from "../lib/queryKeys";
import type { ID, InstanceRole } from "../types";

const adminUsersPageSize = 50;

export function AdminUsersPage() {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [role, setRole] = useState<InstanceRole | "">("");
  const [offset, setOffset] = useState(0);

  const users = useQuery({
    queryKey: [...queryKeys.adminUsers(role, search), "page", offset],
    queryFn: () => api.listAdminUsersPage({ limit: adminUsersPageSize, offset }, { role: role || undefined, q: search || undefined }),
  });

  const updateRole = useMutation({
    mutationFn: ({ userId, instanceRole }: { userId: ID; instanceRole: InstanceRole }) =>
      api.updateAdminUserRole(userId, instanceRole),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.adminUsers(role, search) });
      toast.success(t("admin.roleUpdated"));
    },
    onError: (error) => toast.error(errorMessage(error, t("admin.roleUpdateFailed"))),
  });

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSearch(searchInput.trim());
    setOffset(0);
  }

  return (
    <div className="space-y-5">
      <Panel className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
              <Users size={14} />
              {t("admin.usersBadge")}
            </div>
            <h1 className="text-2xl font-semibold tracking-tight text-zinc-950">{t("admin.usersTitle")}</h1>
          </div>
          <form className="grid gap-3 md:grid-cols-[1fr_180px_auto_auto]" onSubmit={submitSearch}>
            <TextField label={t("admin.search")} value={searchInput} onChange={(event) => setSearchInput(event.target.value)} />
            <SelectField label={t("admin.role")} value={role} onChange={(event) => { setRole(event.target.value as InstanceRole | ""); setOffset(0); }}>
              <option value="">{t("admin.allRoles")}</option>
              {instanceRoles.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.label}
                </option>
              ))}
            </SelectField>
            <Button type="submit" tone="primary" className="self-end">
              <Search size={16} />
              {t("admin.search")}
            </Button>
            <Button onClick={() => users.refetch()} disabled={users.isFetching} className="self-end">
              <RefreshCw size={16} />
              {t("actions.refresh")}
            </Button>
          </form>
        </div>
      </Panel>

      {users.isLoading ? <LoadingState label={t("admin.loadingUsers")} /> : null}
      {users.isError ? <ErrorState title={t("admin.usersLoadFailed")} body={errorMessage(users.error, t("admin.usersRequestFailed"))} /> : null}
      {users.data?.items.length === 0 ? <EmptyState title={t("admin.noUsers")} body={t("admin.noUsersBody")} /> : null}

      {users.data && users.data.items.length > 0 ? (
        <Panel className="overflow-hidden">
          <div className="grid gap-3 border-b border-zinc-100 px-4 py-3 text-xs font-semibold uppercase tracking-wide text-zinc-400 lg:grid-cols-[1.2fr_1fr_220px]">
            <span>{t("admin.user")}</span>
            <span>{t("admin.actor")}</span>
            <span>{t("admin.instanceRole")}</span>
          </div>
          {users.data.items.map((user) => (
            <div key={user.id} className="grid gap-3 border-b border-zinc-100 px-4 py-3 last:border-b-0 lg:grid-cols-[1.2fr_1fr_220px] lg:items-center">
              <div className="min-w-0">
                <div className="font-medium text-zinc-950">{user.username}</div>
                <div className="mt-1 truncate text-sm text-zinc-500">{user.email}</div>
                <div className="mt-1 text-xs text-zinc-400">
                  {compactId(user.id)} · {t("admin.joined", { date: relativeDate(user.created_at) })}
                </div>
              </div>
              <div className="min-w-0 text-sm text-zinc-600">
                <div className="truncate">{user.handle}</div>
                <div className="mt-1 truncate text-xs text-zinc-400">{user.ap_id}</div>
              </div>
              <div className="flex items-end gap-2">
                <SelectField
                  label={t("admin.role")}
                  className="flex-1"
                  value={user.instance_role}
                  onChange={(event) =>
                    updateRole.mutate({ userId: user.id, instanceRole: event.target.value as InstanceRole })
                  }
                >
                  {instanceRoles.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.label}
                    </option>
                  ))}
                </SelectField>
                <div className="mb-0.5 flex h-9 w-9 items-center justify-center rounded-full border border-zinc-200 bg-zinc-50 text-zinc-500">
                  <Shield size={16} />
                </div>
              </div>
            </div>
          ))}
          <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={users.data} onOffsetChange={setOffset} disabled={users.isFetching} /></div>
        </Panel>
      ) : null}
    </div>
  );
}

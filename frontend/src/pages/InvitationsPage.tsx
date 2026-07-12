import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2, Clock3, Mail, RefreshCw, XCircle } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel } from "../components/ui";
import { OffsetPaginationControls } from "../components/OffsetPaginationControls";
import { api, errorMessage } from "../lib/api";
import { initials, relativeDate } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { queryKeys } from "../lib/queryKeys";
import type { ID, ProjectInvite, ProjectInviteInspection, RemoteProjectInvite } from "../types";

type InviteStatusFilter = ProjectInvite["status"] | "";
type InviteBadgeStatus = ProjectInvite["status"] | RemoteProjectInvite["status"];

const statusFilters: InviteStatusFilter[] = ["pending", "accepted", "rejected", "revoked", ""];
const invitationPageSize = 25;

function actorTitle(name: string, username: string, handle: string, fallback: string): string {
  return name || username || handle || fallback;
}

function statusTone(status: InviteBadgeStatus): string {
  switch (status) {
    case "accepted":
      return "border-emerald-200 bg-emerald-50 text-emerald-700";
    case "rejected":
      return "border-zinc-200 bg-zinc-50 text-zinc-500";
    case "revoked":
      return "border-red-200 bg-red-50 text-red-700";
    default:
      return "border-zinc-300 bg-white text-zinc-700";
  }
}

function InviteStatusBadge({ status, label }: { status: InviteBadgeStatus; label: string }) {
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium ${statusTone(status)}`}>{label}</span>;
}

function InviteMetric({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50 px-3 py-2">
      <div className="text-xs text-zinc-400">{label}</div>
      <div className="mt-0.5 font-semibold text-zinc-950">{value}</div>
    </div>
  );
}

function InviteRow({
  invite,
  onAccept,
  onReject,
  pending,
  labels,
}: {
  invite: ProjectInviteInspection;
  onAccept: (inviteId: ID) => void;
  onReject: (inviteId: ID) => void;
  pending: boolean;
  labels: {
    accept: string;
    reject: string;
    open: string;
    role: string;
    status: string;
    unknownActor: string;
  };
}) {
  const inviter = actorTitle(invite.inviter_name, invite.inviter_username, invite.inviter_handle, labels.unknownActor);

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[1fr_auto] lg:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-zinc-50 text-xs font-semibold text-zinc-700">
          {initials(invite.project_name)}
        </div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-zinc-950">{invite.project_name}</span>
            <InviteStatusBadge status={invite.status} label={labels.status} />
          </div>
          <div className="mt-1 truncate text-sm text-zinc-500">{invite.project_handle}</div>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500">
            <span className="inline-flex items-center gap-1">
              <Mail size={12} />
              {inviter}
            </span>
            <span>
              {labels.role}: {invite.role_name || invite.role}
            </span>
            <span>{relativeDate(invite.created_at)}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 lg:justify-end">
        {invite.status === "pending" ? (
          <>
            <Button tone="primary" disabled={pending} onClick={() => onAccept(invite.id)}>
              <CheckCircle2 size={16} />
              {labels.accept}
            </Button>
            <Button tone="danger" disabled={pending} onClick={() => onReject(invite.id)}>
              <XCircle size={16} />
              {labels.reject}
            </Button>
          </>
        ) : null}
        {invite.status === "accepted" ? (
          <Link
            className="focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-full border border-zinc-200 bg-white px-4 text-sm font-medium text-zinc-800 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
            to={`/projects/${invite.project_id}`}
          >
            {labels.open}
            <ArrowRight size={16} />
          </Link>
        ) : null}
      </div>
    </div>
  );
}

function RemoteInviteRow({
  invite,
  onAccept,
  onReject,
  pending,
  labels,
}: {
  invite: RemoteProjectInvite;
  onAccept: (inviteId: ID) => void;
  onReject: (inviteId: ID) => void;
  pending: boolean;
  labels: {
    accept: string;
    reject: string;
    viewFederation: string;
    role: string;
    status: string;
    unknownActor: string;
    remoteProject: string;
  };
}) {
  const inviter = invite.inviter_name || invite.inviter_handle || labels.unknownActor;
  const projectName = invite.project_name || invite.project_ap_id;

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[1fr_auto] lg:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-zinc-50 text-xs font-semibold text-zinc-700">
          {initials(projectName)}
        </div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-zinc-950">{projectName}</span>
            <InviteStatusBadge status={invite.status} label={labels.status} />
          </div>
          <div className="mt-1 truncate text-sm text-zinc-500">{invite.project_ap_id}</div>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500">
            <span className="inline-flex items-center gap-1">
              <Mail size={12} />
              {inviter}
            </span>
            <span>
              {labels.role}: {invite.role || labels.remoteProject}
            </span>
            <span>{relativeDate(invite.created_at)}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 lg:justify-end">
        {invite.status === "pending" ? (
          <>
            <Button tone="primary" disabled={pending} onClick={() => onAccept(invite.id)}>
              <CheckCircle2 size={16} />
              {labels.accept}
            </Button>
            <Button tone="danger" disabled={pending} onClick={() => onReject(invite.id)}>
              <XCircle size={16} />
              {labels.reject}
            </Button>
          </>
        ) : null}
        {invite.status === "accepted" ? (
          <Link
            className="focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-full border border-zinc-200 bg-white px-4 text-sm font-medium text-zinc-800 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
            to="/federation"
          >
            {labels.viewFederation}
            <ArrowRight size={16} />
          </Link>
        ) : null}
      </div>
    </div>
  );
}

export function InvitationsPage() {
  const queryClient = useQueryClient();
  const { t } = useI18n();
  const [status, setStatus] = useState<InviteStatusFilter>("pending");
  const [localOffset, setLocalOffset] = useState(0);
  const [remoteOffset, setRemoteOffset] = useState(0);

  const invites = useQuery({
    queryKey: [...queryKeys.myProjectInvites(status), "page", localOffset],
    queryFn: () => api.listMyProjectInvitesPage({ limit: invitationPageSize, offset: localOffset }, { status }),
  });
  const remoteInvites = useQuery({
    queryKey: [...queryKeys.myRemoteProjectInvites(status), "page", remoteOffset],
    queryFn: () => api.listRemoteProjectInvitesPage({ limit: invitationPageSize, offset: remoteOffset }, { status: status === "revoked" ? "" : status }),
    enabled: status !== "revoked",
  });

  async function refreshInvites() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.myProjectInvitesScope }),
      queryClient.invalidateQueries({ queryKey: queryKeys.myRemoteProjectInvitesScope }),
      queryClient.invalidateQueries({ queryKey: queryKeys.personalFederationFollowsScope }),
      queryClient.invalidateQueries({ queryKey: queryKeys.personalFederationInbox }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projects }),
    ]);
  }

  const acceptInvite = useMutation({
    mutationFn: (inviteId: ID) => api.acceptInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success(t("invitations.accepted"));
    },
    onError: (error) => toast.error(errorMessage(error, t("invitations.acceptFailed"))),
  });

  const rejectInvite = useMutation({
    mutationFn: (inviteId: ID) => api.rejectInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success(t("invitations.rejected"));
    },
    onError: (error) => toast.error(errorMessage(error, t("invitations.rejectFailed"))),
  });

  const acceptRemoteInvite = useMutation({
    mutationFn: (inviteId: ID) => api.acceptRemoteProjectInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success(t("invitations.accepted"));
    },
    onError: (error) => toast.error(errorMessage(error, t("invitations.acceptFailed"))),
  });

  const rejectRemoteInvite = useMutation({
    mutationFn: (inviteId: ID) => api.rejectRemoteProjectInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success(t("invitations.rejected"));
    },
    onError: (error) => toast.error(errorMessage(error, t("invitations.rejectFailed"))),
  });

  const rows = invites.data?.items || [];
  const remoteRows = status === "revoked" ? [] : remoteInvites.data?.items || [];
  const totalRows = rows.length + remoteRows.length;
  const loading = invites.isLoading || (status !== "revoked" && remoteInvites.isLoading);
  const error = invites.error || remoteInvites.error;
  const hasError = invites.isError || remoteInvites.isError;
  const pendingAction = acceptInvite.isPending || rejectInvite.isPending || acceptRemoteInvite.isPending || rejectRemoteInvite.isPending;

  return (
    <div className="space-y-4">
      <Panel className="overflow-hidden">
        <div className="flex flex-col gap-4 p-5 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold text-zinc-950">
              <Mail size={24} />
              {t("invitations.title")}
            </h1>
            <div className="mt-2 flex flex-wrap gap-2 text-sm text-zinc-500">
              <span className="inline-flex items-center gap-1">
                <Clock3 size={14} />
                {status ? t(`status.${status}`) : t("common.all")}
              </span>
              <span>{loading ? "..." : t("common.shown", { count: totalRows })}</span>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2 text-sm sm:flex">
            <InviteMetric label={t("common.filter")} value={status ? t(`status.${status}`) : t("common.all")} />
            <InviteMetric label={t("common.rows")} value={loading ? "..." : totalRows} />
          </div>
        </div>

        <div className="flex flex-col gap-3 border-t border-zinc-100 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap gap-2">
            {statusFilters.map((item) => (
              <button
                key={item || "all"}
                type="button"
                className={`focus-ring rounded-full border px-3 py-1 text-xs font-medium ${
                  status === item ? "border-zinc-950 bg-zinc-950 text-white" : "border-zinc-200 bg-white text-zinc-600 hover:bg-zinc-50"
                }`}
                onClick={() => { setStatus(item); setLocalOffset(0); setRemoteOffset(0); }}
              >
                {item ? t(`status.${item}`) : t("common.all")}
              </button>
            ))}
          </div>
          <Button
            onClick={() => {
              invites.refetch();
              if (status !== "revoked") {
                remoteInvites.refetch();
              }
            }}
            disabled={invites.isFetching || remoteInvites.isFetching}
          >
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
        </div>
      </Panel>

      {loading ? <LoadingState label={t("invitations.loading")} /> : null}
      {hasError ? (
        <ErrorState title={t("invitations.loadFailed")} body={errorMessage(error, t("invitations.loadFailedBody"))} />
      ) : null}
      {!loading && !hasError && totalRows === 0 ? (
        <EmptyState icon={<Mail size={34} />} title={t("invitations.emptyTitle")} body={t("invitations.emptyBody")} />
      ) : null}

      {rows.length > 0 ? (
        <Panel className="overflow-hidden">
          <div className="border-b border-zinc-100 px-4 py-3">
            <h2 className="text-sm font-semibold text-zinc-950">{t("invitations.localTitle")}</h2>
          </div>
          <div className="divide-y divide-zinc-100">
            {rows.map((invite) => (
              <InviteRow
                key={invite.id}
                invite={invite}
                pending={pendingAction}
                onAccept={acceptInvite.mutate}
                onReject={rejectInvite.mutate}
                labels={{
                  accept: t("actions.accept"),
                  reject: t("actions.reject"),
                  open: t("actions.open"),
                  role: t("invitations.role"),
                  status: t(`status.${invite.status}`),
                  unknownActor: t("invitations.unknownActor"),
                }}
              />
            ))}
          </div>
          {invites.data ? <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={invites.data} onOffsetChange={setLocalOffset} disabled={invites.isFetching} /></div> : null}
        </Panel>
      ) : null}

      {remoteRows.length > 0 ? (
        <Panel className="overflow-hidden">
          <div className="border-b border-zinc-100 px-4 py-3">
            <h2 className="text-sm font-semibold text-zinc-950">{t("invitations.remoteTitle")}</h2>
          </div>
          <div className="divide-y divide-zinc-100">
            {remoteRows.map((invite) => (
              <RemoteInviteRow
                key={invite.id}
                invite={invite}
                pending={pendingAction}
                onAccept={acceptRemoteInvite.mutate}
                onReject={rejectRemoteInvite.mutate}
                labels={{
                  accept: t("actions.accept"),
                  reject: t("actions.reject"),
                  viewFederation: t("projects.viewFederation"),
                  role: t("invitations.role"),
                  status: t(`status.${invite.status}`),
                  unknownActor: t("invitations.unknownActor"),
                  remoteProject: t("invitations.remoteProject"),
                }}
              />
            ))}
          </div>
          {remoteInvites.data ? <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={remoteInvites.data} onOffsetChange={setRemoteOffset} disabled={remoteInvites.isFetching} /></div> : null}
        </Panel>
      ) : null}
    </div>
  );
}

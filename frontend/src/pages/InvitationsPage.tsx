import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2, Clock3, Mail, RefreshCw, XCircle } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { initials, relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import type { ID, ProjectInvite, ProjectInviteInspection } from "../types";

type InviteStatusFilter = ProjectInvite["status"] | "";

const statusFilters: InviteStatusFilter[] = ["pending", "accepted", "rejected", "revoked", ""];

function actorTitle(name: string, username: string, handle: string): string {
  return name || username || handle || "Unknown actor";
}

function statusTone(status: ProjectInvite["status"]): string {
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

function InviteStatusBadge({ status }: { status: ProjectInvite["status"] }) {
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium ${statusTone(status)}`}>{status}</span>;
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
}: {
  invite: ProjectInviteInspection;
  onAccept: (inviteId: ID) => void;
  onReject: (inviteId: ID) => void;
  pending: boolean;
}) {
  const inviter = actorTitle(invite.inviter_name, invite.inviter_username, invite.inviter_handle);

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[1fr_auto] lg:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-zinc-50 text-xs font-semibold text-zinc-700">
          {initials(invite.project_name)}
        </div>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-zinc-950">{invite.project_name}</span>
            <InviteStatusBadge status={invite.status} />
          </div>
          <div className="mt-1 truncate text-sm text-zinc-500">{invite.project_handle}</div>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500">
            <span className="inline-flex items-center gap-1">
              <Mail size={12} />
              {inviter}
            </span>
            <span>{invite.role_name || invite.role}</span>
            <span>{relativeDate(invite.created_at)}</span>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 lg:justify-end">
        {invite.status === "pending" ? (
          <>
            <Button tone="primary" disabled={pending} onClick={() => onAccept(invite.id)}>
              <CheckCircle2 size={16} />
              Accept
            </Button>
            <Button tone="danger" disabled={pending} onClick={() => onReject(invite.id)}>
              <XCircle size={16} />
              Reject
            </Button>
          </>
        ) : null}
        {invite.status === "accepted" ? (
          <Link
            className="focus-ring inline-flex h-9 items-center justify-center gap-2 rounded-full border border-zinc-200 bg-white px-4 text-sm font-medium text-zinc-800 shadow-sm transition hover:border-zinc-300 hover:bg-zinc-50"
            to={`/projects/${invite.project_id}`}
          >
            Open
            <ArrowRight size={16} />
          </Link>
        ) : null}
      </div>
    </div>
  );
}

export function InvitationsPage() {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<InviteStatusFilter>("pending");

  const invites = useQuery({
    queryKey: queryKeys.myProjectInvites(status),
    queryFn: () => api.listMyProjectInvites({ status }),
  });

  async function refreshInvites() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.myProjectInvitesScope }),
      queryClient.invalidateQueries({ queryKey: queryKeys.projects }),
    ]);
  }

  const acceptInvite = useMutation({
    mutationFn: (inviteId: ID) => api.acceptInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success("Invitation accepted");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not accept invitation.")),
  });

  const rejectInvite = useMutation({
    mutationFn: (inviteId: ID) => api.rejectInvite(inviteId),
    onSuccess: async () => {
      await refreshInvites();
      toast.success("Invitation rejected");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not reject invitation.")),
  });

  const rows = invites.data || [];
  const pendingAction = acceptInvite.isPending || rejectInvite.isPending;

  return (
    <div className="space-y-4">
      <Panel className="overflow-hidden">
        <div className="flex flex-col gap-4 p-5 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold text-zinc-950">
              <Mail size={24} />
              Invitations
            </h1>
            <div className="mt-2 flex flex-wrap gap-2 text-sm text-zinc-500">
              <span className="inline-flex items-center gap-1">
                <Clock3 size={14} />
                {status || "all"}
              </span>
              <span>{invites.isLoading ? "..." : `${rows.length} shown`}</span>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2 text-sm sm:flex">
            <InviteMetric label="Filter" value={status || "all"} />
            <InviteMetric label="Rows" value={invites.isLoading ? "..." : rows.length} />
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
                onClick={() => setStatus(item)}
              >
                {item || "all"}
              </button>
            ))}
          </div>
          <Button onClick={() => invites.refetch()} disabled={invites.isFetching}>
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
      </Panel>

      {invites.isLoading ? <LoadingState label="Loading invitations" /> : null}
      {invites.isError ? (
        <ErrorState title="Could not load invitations" body={errorMessage(invites.error, "Invitation request failed.")} />
      ) : null}
      {!invites.isLoading && !invites.isError && rows.length === 0 ? (
        <EmptyState icon={<Mail size={34} />} title="No invitations" body="Project invitations addressed to this account will appear here." />
      ) : null}

      {rows.length > 0 ? (
        <Panel className="overflow-hidden">
          <div className="divide-y divide-zinc-100">
            {rows.map((invite) => (
              <InviteRow
                key={invite.id}
                invite={invite}
                pending={pendingAction}
                onAccept={acceptInvite.mutate}
                onReject={rejectInvite.mutate}
              />
            ))}
          </div>
        </Panel>
      ) : null}
    </div>
  );
}

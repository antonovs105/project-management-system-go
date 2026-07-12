import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, RotateCcw } from "lucide-react";
import { useState } from "react";
import { OffsetPaginationControls } from "../../components/OffsetPaginationControls";
import { Button, EmptyState, ErrorState, LoadingState, Panel } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { useI18n } from "../../lib/i18n-context";
import { queryKeys } from "../../lib/queryKeys";
import type { ID } from "../../types";

const activityPageSize = 25;

export function ProjectActivityPanel({ projectId }: { projectId: ID }) {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [offset, setOffset] = useState(0);
  const events = useQuery({
    queryKey: [...queryKeys.projectActivity(projectId), "page", offset],
    queryFn: () => api.listProjectActivityPage(projectId, { limit: activityPageSize, offset }),
  });
  const archivedTickets = useQuery({ queryKey: queryKeys.archivedTickets(projectId), queryFn: () => api.listArchivedTickets(projectId) });
  const restore = useMutation({
    mutationFn: ({ id, version }: { id: ID; version: number }) => api.restoreTicket(id, version),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.archivedTickets(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.projectActivity(projectId) }),
      ]);
    },
  });

  if (events.isLoading || archivedTickets.isLoading) return <LoadingState label={t("project.activityLoading")} />;
  if (events.isError || archivedTickets.isError) return <ErrorState title={t("project.activityLoadFailed")} body={errorMessage(events.error || archivedTickets.error, t("project.activityRequestFailed"))} />;
  if (!events.data?.items.length && !archivedTickets.data?.length) return <EmptyState title={t("project.activityEmpty")} body={t("project.activityEmptyBody")} />;

  return (
    <div className="grid gap-4">
      {(archivedTickets.data || []).length > 0 ? (
        <Panel className="divide-y divide-zinc-100">
          <div className="p-4 font-semibold">{t("project.archivedTickets")}</div>
          {archivedTickets.data?.map((ticket) => (
            <div key={ticket.id} className="flex items-center justify-between gap-3 p-4">
              <div><div className="text-sm font-medium">{ticket.title}</div><div className="text-xs text-zinc-500">{t("project.archivedAt", { date: relativeDate(ticket.archived_at) })}</div></div>
              <Button onClick={() => restore.mutate({ id: ticket.id, version: ticket.version })}><RotateCcw size={15} />{t("actions.restore")}</Button>
            </div>
          ))}
        </Panel>
      ) : null}
      <Panel className="divide-y divide-zinc-200">
        {(events.data?.items || []).map((event) => (
          <div key={event.id} className="flex gap-3 p-4"><Activity size={17} className="mt-0.5 text-zinc-400" /><div><div className="text-sm text-zinc-950"><span className="font-medium">{event.actor_handle || t("project.system")}</span> {event.action} {event.entity_type}</div><div className="mt-1 text-xs text-zinc-500">{relativeDate(event.created_at)} · {event.entity_id}</div></div></div>
        ))}
        <div className="p-4"><OffsetPaginationControls page={events.data!} onOffsetChange={setOffset} disabled={events.isFetching} /></div>
      </Panel>
    </div>
  );
}

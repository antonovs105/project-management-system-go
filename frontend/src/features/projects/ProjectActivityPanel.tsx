import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, RotateCcw } from "lucide-react";
import { Button, EmptyState, ErrorState, LoadingState, Panel } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type { ID } from "../../types";

export function ProjectActivityPanel({ projectId }: { projectId: ID }) {
	const queryClient = useQueryClient();
	const events = useQuery({ queryKey: queryKeys.projectActivity(projectId), queryFn: () => api.listProjectActivity(projectId) });
	const archivedTickets = useQuery({ queryKey: queryKeys.archivedTickets(projectId), queryFn: () => api.listArchivedTickets(projectId) });
	const restore = useMutation({ mutationFn: ({ id, version }: { id: ID; version: number }) => api.restoreTicket(id, version), onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: queryKeys.archivedTickets(projectId) }), queryClient.invalidateQueries({ queryKey: queryKeys.ticketsScope(projectId) }), queryClient.invalidateQueries({ queryKey: queryKeys.projectActivity(projectId) })]); } });
	if (events.isLoading || archivedTickets.isLoading) return <LoadingState label="Loading activity" />;
	if (events.isError || archivedTickets.isError) return <ErrorState title="Could not load activity" body={errorMessage(events.error || archivedTickets.error, "Activity request failed.")} />;
	if (!events.data?.length && !archivedTickets.data?.length) return <EmptyState title="No activity yet" body="Project and ticket changes will appear here." />;
	return <div className="grid gap-4">{(archivedTickets.data || []).length > 0 ? <Panel className="divide-y divide-zinc-100"><div className="p-4 font-semibold">Archived tickets</div>{archivedTickets.data?.map((ticket) => <div key={ticket.id} className="flex items-center justify-between gap-3 p-4"><div><div className="text-sm font-medium">{ticket.title}</div><div className="text-xs text-zinc-500">Archived {relativeDate(ticket.archived_at)}</div></div><Button onClick={() => restore.mutate({ id: ticket.id, version: ticket.version })}><RotateCcw size={15} />Restore</Button></div>)}</Panel> : null}<Panel className="divide-y divide-zinc-200">{(events.data || []).map((event) => <div key={event.id} className="flex gap-3 p-4"><Activity size={17} className="mt-0.5 text-zinc-400" /><div><div className="text-sm text-zinc-950"><span className="font-medium">{event.actor_handle || "System"}</span> {event.action} a {event.entity_type}</div><div className="mt-1 text-xs text-zinc-500">{relativeDate(event.created_at)} · {event.entity_id}</div></div></div>)}</Panel></div>;
}

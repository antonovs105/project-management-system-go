import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Search, ScrollText } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { Button, EmptyState, ErrorState, LoadingState, Panel, SelectField, TextField } from "../components/ui";
import { OffsetPaginationControls } from "../components/OffsetPaginationControls";
import { api, errorMessage } from "../lib/api";
import { adminAuditActions, adminAuditTargetTypes } from "../lib/constants";
import { compactId, relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import type { AdminAuditAction, AdminAuditTargetType } from "../types";

const auditPageSize = 50;

function metadataPreview(metadata: Record<string, unknown>): string {
  const text = JSON.stringify(metadata);
  if (text.length <= 180) {
    return text;
  }
  return `${text.slice(0, 180)}...`;
}

export function AdminAuditPage() {
  const [actionInput, setActionInput] = useState<AdminAuditAction | "">("");
  const [targetTypeInput, setTargetTypeInput] = useState<AdminAuditTargetType | "">("");
  const [actorInput, setActorInput] = useState("");
  const [action, setAction] = useState<AdminAuditAction | "">("");
  const [targetType, setTargetType] = useState<AdminAuditTargetType | "">("");
  const [actorUserId, setActorUserId] = useState("");
  const [offset, setOffset] = useState(0);

  const events = useQuery({
    queryKey: [...queryKeys.adminAuditEvents(action, targetType, actorUserId), "page", offset],
    queryFn: () =>
      api.listAdminAuditEventsPage({ limit: auditPageSize, offset }, {
        action: action || undefined,
        target_type: targetType || undefined,
        actor_user_id: actorUserId || undefined,
      }),
  });

  function submitFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setAction(actionInput);
    setTargetType(targetTypeInput);
    setActorUserId(actorInput.trim());
    setOffset(0);
  }

  return (
    <div className="space-y-5">
      <Panel className="p-5">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
              <ScrollText size={14} />
              Instance audit
            </div>
            <h1 className="text-2xl font-semibold tracking-tight text-zinc-950">Audit Events</h1>
          </div>
          <form className="grid gap-3 md:grid-cols-[190px_190px_1fr_auto_auto]" onSubmit={submitFilters}>
            <SelectField label="Action" value={actionInput} onChange={(event) => setActionInput(event.target.value as AdminAuditAction | "")}>
              <option value="">All actions</option>
              {adminAuditActions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.label}
                </option>
              ))}
            </SelectField>
            <SelectField
              label="Target"
              value={targetTypeInput}
              onChange={(event) => setTargetTypeInput(event.target.value as AdminAuditTargetType | "")}
            >
              <option value="">All targets</option>
              {adminAuditTargetTypes.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.label}
                </option>
              ))}
            </SelectField>
            <TextField label="Actor user ID" value={actorInput} onChange={(event) => setActorInput(event.target.value)} />
            <Button type="submit" tone="primary" className="self-end">
              <Search size={16} />
              Filter
            </Button>
            <Button onClick={() => events.refetch()} disabled={events.isFetching} className="self-end">
              <RefreshCw size={16} />
              Refresh
            </Button>
          </form>
        </div>
      </Panel>

      {events.isLoading ? <LoadingState label="Loading audit events" /> : null}
      {events.isError ? (
        <ErrorState title="Could not load audit events" body={errorMessage(events.error, "Audit event request failed.")} />
      ) : null}
      {events.data?.items.length === 0 ? <EmptyState title="No audit events" body="No audit events match the current filters." /> : null}

      {events.data && events.data.items.length > 0 ? (
        <Panel className="overflow-hidden">
          {events.data.items.map((event) => (
            <div key={event.id} className="grid gap-3 border-b border-zinc-100 px-4 py-3 last:border-b-0 xl:grid-cols-[1fr_1fr_1.4fr]">
              <div className="min-w-0">
                <div className="font-medium text-zinc-950">{event.action}</div>
                <div className="mt-1 text-xs text-zinc-500">
                  {compactId(event.id)} · {relativeDate(event.created_at)}
                </div>
              </div>
              <div className="min-w-0 text-sm text-zinc-600">
                <div>{event.target_type}</div>
                <div className="mt-1 truncate text-xs text-zinc-500">{event.target_id}</div>
                {event.actor_user_id ? <div className="mt-1 truncate text-xs text-zinc-400">actor {event.actor_user_id}</div> : null}
              </div>
              <code className="block min-w-0 whitespace-pre-wrap rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-700">
                {metadataPreview(event.metadata)}
              </code>
            </div>
          ))}
          <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={events.data} onOffsetChange={setOffset} disabled={events.isFetching} /></div>
        </Panel>
      ) : null}
    </div>
  );
}

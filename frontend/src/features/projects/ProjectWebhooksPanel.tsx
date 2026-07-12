import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clipboard, Radio, RefreshCw, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { relativeDate } from "../../lib/format";
import { queryKeys } from "../../lib/queryKeys";
import type { ID, WebhookEvent } from "../../types";

const webhookEvents: Array<{ event: WebhookEvent; label: string }> = [
  { event: "project.updated", label: "Project updated" },
  { event: "project.archived", label: "Project archived" },
  { event: "project.restored", label: "Project restored" },
  { event: "ticket.created", label: "Ticket created" },
  { event: "ticket.updated", label: "Ticket updated" },
  { event: "ticket.archived", label: "Ticket archived" },
  { event: "ticket.restored", label: "Ticket restored" },
];

export function ProjectWebhooksPanel({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [targetURL, setTargetURL] = useState("");
  const [events, setEvents] = useState<WebhookEvent[]>(["ticket.created", "ticket.updated"]);
  const [createdSecret, setCreatedSecret] = useState("");
  const webhooks = useQuery({ queryKey: queryKeys.projectWebhooks(projectId), queryFn: () => api.listProjectWebhooks(projectId) });
  const deliveries = useQuery({ queryKey: queryKeys.projectWebhookDeliveries(projectId), queryFn: () => api.listProjectWebhookDeliveries(projectId) });
  const refresh = async () => Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.projectWebhooks(projectId) }),
    queryClient.invalidateQueries({ queryKey: queryKeys.projectWebhookDeliveries(projectId) }),
  ]);
  const createWebhook = useMutation({
    mutationFn: () => api.createProjectWebhook(projectId, { name: name.trim(), target_url: targetURL.trim(), events }),
    onSuccess: async (created) => {
      setCreatedSecret(created.secret);
      setName("");
      setTargetURL("");
      await refresh();
    },
    onError: (error) => toast.error(errorMessage(error, "Could not create outbound webhook.")),
  });
  const deleteWebhook = useMutation({
    mutationFn: (webhookId: ID) => api.deleteProjectWebhook(projectId, webhookId),
    onSuccess: refresh,
    onError: (error) => toast.error(errorMessage(error, "Could not delete outbound webhook.")),
  });
  const retryDelivery = useMutation({
    mutationFn: (deliveryId: ID) => api.retryProjectWebhookDelivery(projectId, deliveryId),
    onSuccess: refresh,
    onError: (error) => toast.error(errorMessage(error, "Could not retry webhook delivery.")),
  });

  function toggleEvent(event: WebhookEvent, enabled: boolean) {
    setEvents((current) => enabled ? [...new Set([...current, event])] : current.filter((value) => value !== event));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    createWebhook.mutate();
  }

  return (
    <Panel className="overflow-hidden">
      <div className="border-b border-zinc-100 p-4"><h2 className="flex items-center gap-2 font-semibold text-zinc-950"><Radio size={17} />Outbound webhooks</h2><p className="mt-1 text-sm text-zinc-500">Deliver signed project events with durable retries. Verify X-Progo-Signature-256 before processing a payload.</p></div>
      <div className="grid gap-5 p-4 xl:grid-cols-[0.9fr_1.1fr]">
        <form className="grid content-start gap-4" onSubmit={submit}>
          {createdSecret ? <div className="rounded-xl border border-amber-300 bg-amber-50 p-3" role="status"><div className="font-semibold text-amber-950">Copy the signing secret now</div><code className="mt-2 block break-all rounded-lg bg-white p-2 text-sm">{createdSecret}</code><div className="mt-2 flex gap-2"><Button onClick={() => void navigator.clipboard.writeText(createdSecret)}><Clipboard size={15} />Copy</Button><Button onClick={() => setCreatedSecret("")}>I saved it</Button></div></div> : null}
          <TextField label="Webhook name" value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required />
          <TextField label="HTTPS target URL" type="url" value={targetURL} onChange={(event) => setTargetURL(event.target.value)} placeholder="https://automation.example/hooks/progo" required />
          <fieldset><legend className="mb-2 text-sm font-medium text-zinc-700">Events</legend><div className="grid gap-2 sm:grid-cols-2">{webhookEvents.map((item) => <label key={item.event} className="flex items-center gap-2 rounded-xl border border-zinc-200 px-3 py-2 text-sm"><input type="checkbox" checked={events.includes(item.event)} onChange={(change) => toggleEvent(item.event, change.target.checked)} />{item.label}</label>)}</div></fieldset>
          <Button type="submit" tone="primary" disabled={!name.trim() || !targetURL.trim() || events.length === 0 || createWebhook.isPending}>Create webhook</Button>
        </form>
        <div className="grid content-start gap-4">
          {webhooks.isError ? <ErrorState title="Could not load webhooks" body={errorMessage(webhooks.error, "Webhook request failed.")} /> : null}
          <div className="grid gap-2">{(webhooks.data || []).map((webhook) => <div key={webhook.id} className="flex items-start justify-between gap-3 rounded-xl border border-zinc-200 p-3"><div className="min-w-0"><div className="font-medium text-zinc-950">{webhook.name}</div><div className="truncate text-xs text-zinc-500">{webhook.target_url}</div><div className="mt-1 text-xs text-zinc-500">{webhook.events.join(", ")}</div></div><Button tone="danger" onClick={() => deleteWebhook.mutate(webhook.id)} disabled={deleteWebhook.isPending}><Trash2 size={15} />Delete</Button></div>)}</div>
          <div><div className="mb-2 flex items-center justify-between"><h3 className="font-semibold text-zinc-950">Recent deliveries</h3><Button onClick={() => void refresh()}><RefreshCw size={15} />Refresh</Button></div>{deliveries.isError ? <ErrorState title="Could not load deliveries" body={errorMessage(deliveries.error, "Delivery request failed.")} /> : null}<div className="grid gap-2">{(deliveries.data || []).map((delivery) => <div key={delivery.id} className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 p-3"><div><div className="text-sm font-medium text-zinc-950">{delivery.event_type} · {delivery.status}</div><div className="text-xs text-zinc-500">{delivery.webhook_name} · {delivery.attempts}/{delivery.max_attempts} attempts · {relativeDate(delivery.created_at)}</div>{delivery.last_error ? <div className="mt-1 text-xs text-red-700">{delivery.last_error}</div> : null}</div>{delivery.status === "failed" || delivery.status === "dead" ? <Button onClick={() => retryDelivery.mutate(delivery.id)} disabled={retryDelivery.isPending}>Retry</Button> : null}</div>)}</div></div>
        </div>
      </div>
    </Panel>
  );
}

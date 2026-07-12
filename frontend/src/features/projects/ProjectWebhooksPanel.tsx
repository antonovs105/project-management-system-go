import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clipboard, Radio, RefreshCw, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button, ErrorState, Panel, TextField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { useI18n } from "../../lib/i18n-context";
import { queryKeys } from "../../lib/queryKeys";
import type { ID, WebhookEvent } from "../../types";

export function ProjectWebhooksPanel({ projectId }: { projectId: ID }) {
  const { t, relativeDate } = useI18n();
  const webhookEvents: Array<{ event: WebhookEvent; label: string }> = [
    { event: "project.updated", label: t("webhooks.projectUpdated") },
    { event: "project.archived", label: t("webhooks.projectArchived") },
    { event: "project.restored", label: t("webhooks.projectRestored") },
    { event: "ticket.created", label: t("webhooks.ticketCreated") },
    { event: "ticket.updated", label: t("webhooks.ticketUpdated") },
    { event: "ticket.archived", label: t("webhooks.ticketArchived") },
    { event: "ticket.restored", label: t("webhooks.ticketRestored") },
  ];
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
    onError: (error) => toast.error(errorMessage(error, t("webhooks.createFailed"))),
  });
  const deleteWebhook = useMutation({
    mutationFn: (webhookId: ID) => api.deleteProjectWebhook(projectId, webhookId),
    onSuccess: refresh,
    onError: (error) => toast.error(errorMessage(error, t("webhooks.deleteFailed"))),
  });
  const retryDelivery = useMutation({
    mutationFn: (deliveryId: ID) => api.retryProjectWebhookDelivery(projectId, deliveryId),
    onSuccess: refresh,
    onError: (error) => toast.error(errorMessage(error, t("webhooks.retryFailed"))),
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
      <div className="border-b border-zinc-100 p-4"><h2 className="flex items-center gap-2 font-semibold text-zinc-950"><Radio size={17} />{t("webhooks.title")}</h2><p className="mt-1 text-sm text-zinc-500">{t("webhooks.body")}</p></div>
      <div className="grid gap-5 p-4 xl:grid-cols-[0.9fr_1.1fr]">
        <form className="grid content-start gap-4" onSubmit={submit}>
          {createdSecret ? <div className="rounded-xl border border-amber-300 bg-amber-50 p-3" role="status"><div className="font-semibold text-amber-950">{t("webhooks.copySecret")}</div><code className="mt-2 block break-all rounded-lg bg-white p-2 text-sm">{createdSecret}</code><div className="mt-2 flex gap-2"><Button onClick={() => void navigator.clipboard.writeText(createdSecret)}><Clipboard size={15} />{t("actions.copy")}</Button><Button onClick={() => setCreatedSecret("")}>{t("webhooks.saved")}</Button></div></div> : null}
          <TextField label={t("webhooks.name")} value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required />
          <TextField label={t("webhooks.target")} type="url" value={targetURL} onChange={(event) => setTargetURL(event.target.value)} placeholder="https://automation.example/hooks/progo" required />
          <fieldset><legend className="mb-2 text-sm font-medium text-zinc-700">{t("webhooks.events")}</legend><div className="grid gap-2 sm:grid-cols-2">{webhookEvents.map((item) => <label key={item.event} className="flex items-center gap-2 rounded-xl border border-zinc-200 px-3 py-2 text-sm"><input type="checkbox" checked={events.includes(item.event)} onChange={(change) => toggleEvent(item.event, change.target.checked)} />{item.label}</label>)}</div></fieldset>
          <Button type="submit" tone="primary" disabled={!name.trim() || !targetURL.trim() || events.length === 0 || createWebhook.isPending}>{t("webhooks.create")}</Button>
        </form>
        <div className="grid content-start gap-4">
          {webhooks.isError ? <ErrorState title={t("webhooks.loadFailed")} body={errorMessage(webhooks.error, t("webhooks.requestFailed"))} /> : null}
          <div className="grid gap-2">{(webhooks.data || []).map((webhook) => <div key={webhook.id} className="flex items-start justify-between gap-3 rounded-xl border border-zinc-200 p-3"><div className="min-w-0"><div className="font-medium text-zinc-950">{webhook.name}</div><div className="truncate text-xs text-zinc-500">{webhook.target_url}</div><div className="mt-1 text-xs text-zinc-500">{webhook.events.join(", ")}</div></div><Button tone="danger" onClick={() => deleteWebhook.mutate(webhook.id)} disabled={deleteWebhook.isPending}><Trash2 size={15} />{t("actions.delete")}</Button></div>)}</div>
          <div><div className="mb-2 flex items-center justify-between"><h3 className="font-semibold text-zinc-950">{t("webhooks.recentDeliveries")}</h3><Button onClick={() => void refresh()}><RefreshCw size={15} />{t("actions.refresh")}</Button></div>{deliveries.isError ? <ErrorState title={t("webhooks.deliveriesLoadFailed")} body={errorMessage(deliveries.error, t("webhooks.deliveryRequestFailed"))} /> : null}<div className="grid gap-2">{(deliveries.data || []).map((delivery) => <div key={delivery.id} className="flex items-center justify-between gap-3 rounded-xl border border-zinc-200 p-3"><div><div className="text-sm font-medium text-zinc-950">{delivery.event_type} · {delivery.status}</div><div className="text-xs text-zinc-500">{delivery.webhook_name} · {t("webhooks.attempts", { attempts: delivery.attempts, max: delivery.max_attempts })} · {relativeDate(delivery.created_at)}</div>{delivery.last_error ? <div className="mt-1 text-xs text-red-700">{delivery.last_error}</div> : null}</div>{delivery.status === "failed" || delivery.status === "dead" ? <Button onClick={() => retryDelivery.mutate(delivery.id)} disabled={retryDelivery.isPending}>{t("webhooks.retry")}</Button> : null}</div>)}</div></div>
        </div>
      </div>
    </Panel>
  );
}

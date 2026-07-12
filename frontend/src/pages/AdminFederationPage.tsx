import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Globe2, RadioTower, RefreshCw, RotateCcw, Trash2 } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel, SelectField, TextAreaField, TextField } from "../components/ui";
import { OffsetPaginationControls } from "../components/OffsetPaginationControls";
import { api, errorMessage } from "../lib/api";
import { deliveryFailureKinds, deliveryStates } from "../lib/constants";
import { compactId } from "../lib/format";
import { useI18n } from "../lib/i18n-context";
import { queryKeys } from "../lib/queryKeys";
import type { DeliveryFailureKind, DeliveryState, FederationDelivery, ID } from "../types";

function deliveryStateClass(state: DeliveryState): string {
  switch (state) {
    case "delivered":
      return "border-zinc-950 bg-zinc-950 text-white";
    case "dead":
      return "border-red-200 bg-red-50 text-red-700";
    case "failed":
      return "border-zinc-300 bg-zinc-200 text-zinc-950";
    case "processing":
      return "border-zinc-300 bg-white text-zinc-950";
    default:
      return "border-zinc-200 bg-zinc-50 text-zinc-500";
  }
}

function FederationDeliveryRow({
  delivery,
  onRetry,
  retrying,
}: {
  delivery: FederationDelivery;
  onRetry: (deliveryId: ID) => void;
  retrying: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="grid gap-3 border-t border-zinc-100 px-4 py-3 xl:grid-cols-[1.1fr_1.1fr_0.8fr_auto] xl:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`rounded-full border px-2 py-0.5 text-xs font-medium ${deliveryStateClass(delivery.state)}`}>
            {delivery.state}
          </span>
          <span className="font-medium text-zinc-950">{delivery.activity_type}</span>
        </div>
        <div className="mt-1 truncate text-xs text-zinc-500">{delivery.actor_ap_id}</div>
      </div>
      <div className="min-w-0 text-sm text-zinc-600">
        <div className="truncate">{delivery.target_inbox_url}</div>
        {delivery.project_id ? <div className="mt-1 text-xs text-zinc-500">{t("admin.deliveryProject", { id: compactId(delivery.project_id) })}</div> : null}
      </div>
      <div className="text-sm text-zinc-600">
        <div>
          {t("admin.deliveryAttempts", { attempts: delivery.attempts, max: delivery.max_attempts })}
        </div>
        {delivery.last_failure_kind ? <div className="mt-1 text-xs text-zinc-500">{delivery.last_failure_kind}</div> : null}
        {delivery.last_error ? <div className="mt-1 line-clamp-2 text-xs text-red-700">{delivery.last_error}</div> : null}
      </div>
      <div className="flex items-center justify-between gap-2 xl:justify-end">
        <span className="rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">
          {compactId(delivery.id)}
        </span>
        <Button onClick={() => onRetry(delivery.id)} disabled={!delivery.can_retry || retrying}>
          <RotateCcw size={15} />
          {t("admin.retry")}
        </Button>
      </div>
    </div>
  );
}

function DomainBlocksPanel() {
  const { t, relativeDate } = useI18n();
  const queryClient = useQueryClient();
  const [domain, setDomain] = useState("");
  const [reason, setReason] = useState("");

  const blocks = useQuery({
    queryKey: queryKeys.federationDomainBlocks,
    queryFn: api.listFederationDomainBlocks,
  });

  const blockDomain = useMutation({
    mutationFn: () => api.blockFederationDomain({ domain: domain.trim(), reason: reason.trim() || undefined }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.federationDomainBlocks }),
        queryClient.invalidateQueries({ queryKey: queryKeys.adminAuditEvents() }),
      ]);
      setDomain("");
      setReason("");
      toast.success(t("admin.domainBlocked"));
    },
    onError: (error) => toast.error(errorMessage(error, t("admin.domainBlockFailed"))),
  });

  const unblockDomain = useMutation({
    mutationFn: (blockedDomain: string) => api.unblockFederationDomain(blockedDomain),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.federationDomainBlocks }),
        queryClient.invalidateQueries({ queryKey: queryKeys.adminAuditEvents() }),
      ]);
      toast.success(t("admin.domainUnblocked"));
    },
    onError: (error) => toast.error(errorMessage(error, t("admin.domainUnblockFailed"))),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    blockDomain.mutate();
  }

  return (
    <Panel className="overflow-hidden">
      <div className="px-4 py-4">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <Ban size={17} />
          {t("admin.domainBlocks")}
        </h2>
      </div>
      <form className="grid gap-3 border-t border-zinc-100 p-4 md:grid-cols-[1fr_1.5fr_auto]" onSubmit={submit}>
        <TextField label={t("admin.domain")} value={domain} onChange={(event) => setDomain(event.target.value)} required />
        <TextAreaField label={t("admin.reason")} value={reason} onChange={(event) => setReason(event.target.value)} className="md:row-span-2" />
        <Button type="submit" tone="primary" className="self-end" disabled={blockDomain.isPending || !domain.trim()}>
          {t("admin.block")}
        </Button>
      </form>

      {blocks.isLoading ? <LoadingState label={t("admin.loadingDomainBlocks")} /> : null}
      {blocks.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("admin.domainBlocksLoadFailed")} body={errorMessage(blocks.error, t("admin.domainBlocksRequestFailed"))} />
        </div>
      ) : null}
      {blocks.data?.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title={t("admin.noDomainBlocks")} body={t("admin.noDomainBlocksBody")} />
        </div>
      ) : null}
      {blocks.data?.map((block) => (
        <div key={block.id} className="grid gap-3 border-t border-zinc-100 px-4 py-3 md:grid-cols-[1fr_1.5fr_auto] md:items-center">
          <div className="min-w-0">
            <div className="font-medium text-zinc-950">{block.domain}</div>
            <div className="mt-1 text-xs text-zinc-500">{relativeDate(block.created_at)}</div>
          </div>
          <div className="min-w-0 text-sm text-zinc-600">{block.reason || t("admin.noReason")}</div>
          <Button tone="danger" onClick={() => unblockDomain.mutate(block.domain)} disabled={unblockDomain.isPending}>
            <Trash2 size={15} />
            {t("admin.unblock")}
          </Button>
        </div>
      ))}
    </Panel>
  );
}

function RemoteActorsPanel() {
  const { t, relativeDate } = useI18n();
  const [fetchErrorsOnly, setFetchErrorsOnly] = useState(false);
  const [offset, setOffset] = useState(0);
  const actors = useQuery({
    queryKey: queryKeys.federationRemoteActors(fetchErrorsOnly ? "errors" : "all", offset),
    queryFn: () => api.listFederationRemoteActorsPage({ limit: 50, offset }, { fetch_error: fetchErrorsOnly || undefined }),
  });

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-end md:justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <Globe2 size={17} />
          {t("admin.remoteActors")}
        </h2>
        <div className="flex gap-2">
          <SelectField
            label={t("admin.fetchState")}
            value={fetchErrorsOnly ? "errors" : ""}
            onChange={(event) => {
              setFetchErrorsOnly(event.target.value === "errors");
              setOffset(0);
            }}
          >
            <option value="">{t("admin.allActors")}</option>
            <option value="errors">{t("admin.fetchErrors")}</option>
          </SelectField>
          <Button onClick={() => actors.refetch()} disabled={actors.isFetching} className="self-end">
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
        </div>
      </div>

      {actors.isLoading ? <LoadingState label={t("admin.loadingActors")} /> : null}
      {actors.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("admin.actorsLoadFailed")} body={errorMessage(actors.error, t("admin.actorsRequestFailed"))} />
        </div>
      ) : null}
      {actors.data?.items.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title={t("admin.noActors")} body={t("admin.noActorsBody")} />
        </div>
      ) : null}
      {actors.data?.items.map((actor) => (
        <div key={actor.id} className="grid gap-3 border-t border-zinc-100 px-4 py-3 lg:grid-cols-[1fr_1.3fr_1fr]">
          <div className="min-w-0">
            <div className="font-medium text-zinc-950">{actor.handle || actor.preferred_username}</div>
            <div className="mt-1 text-xs text-zinc-500">{actor.type}</div>
          </div>
          <div className="min-w-0 text-sm text-zinc-600">
            <div className="truncate">{actor.ap_id}</div>
            <div className="mt-1 truncate text-xs text-zinc-400">{actor.inbox_url}</div>
          </div>
          <div className="min-w-0 text-sm text-zinc-600">
            {actor.fetch_error ? <div className="line-clamp-2 text-red-700">{actor.fetch_error}</div> : <div>{t("admin.fetched")}</div>}
            <div className="mt-1 text-xs text-zinc-400">
              {actor.last_fetched_at ? relativeDate(actor.last_fetched_at) : relativeDate(actor.created_at)}
            </div>
          </div>
        </div>
      ))}
      {actors.data ? (
        <div className="border-t border-zinc-100 p-4">
          <OffsetPaginationControls page={actors.data} onOffsetChange={setOffset} disabled={actors.isFetching} />
        </div>
      ) : null}
    </Panel>
  );
}

function FederationDeliveriesPanel() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [stateFilter, setStateFilter] = useState<DeliveryState | "">("");
  const [failureKind, setFailureKind] = useState<DeliveryFailureKind | "">("");
  const [offset, setOffset] = useState(0);

  const summary = useQuery({
    queryKey: queryKeys.federationDeliverySummary,
    queryFn: api.getFederationDeliverySummary,
  });

  const deliveries = useQuery({
    queryKey: queryKeys.federationDeliveries(stateFilter, failureKind, offset),
    queryFn: () =>
      api.listFederationDeliveriesPage({ limit: 50, offset }, {
        state: stateFilter || undefined,
        failure_kind: failureKind || undefined,
      }),
  });

  const retryDelivery = useMutation({
    mutationFn: api.retryFederationDelivery,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.federationDeliveries(stateFilter, failureKind) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.federationDeliverySummary }),
        queryClient.invalidateQueries({ queryKey: queryKeys.adminAuditEvents() }),
      ]);
      toast.success(t("admin.deliveryQueued"));
    },
    onError: (error) => toast.error(errorMessage(error, t("admin.deliveryRetryFailed"))),
  });

  const summaryItems = summary.data
    ? [
        [t("admin.total"), summary.data.total],
        [t("admin.pending"), summary.data.pending],
        [t("admin.failed"), summary.data.failed],
        [t("admin.dead"), summary.data.dead],
        [t("admin.retryable"), summary.data.retryable],
        [t("admin.dueRetry"), summary.data.due_retry],
        [t("admin.http"), summary.data.http_failures],
        [t("admin.network"), summary.data.network_failures],
      ]
    : [];

  return (
    <Panel className="overflow-hidden">
      <div className="px-4 py-4">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <RadioTower size={17} />
          {t("admin.deliveries")}
        </h2>
      </div>

      {summary.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("admin.deliverySummaryFailed")} body={errorMessage(summary.error, t("admin.summaryRequestFailed"))} />
        </div>
      ) : null}

      <div className="grid gap-3 border-t border-zinc-100 p-4 sm:grid-cols-2 xl:grid-cols-4">
        {summary.isLoading ? <LoadingState label={t("admin.loadingDeliverySummary")} /> : null}
        {summaryItems.map(([label, value]) => (
          <div key={label} className="rounded-xl border border-zinc-200 p-3">
            <div className="text-xs font-medium uppercase tracking-wide text-zinc-400">{label}</div>
            <div className="mt-2 text-xl font-semibold text-zinc-950">{value}</div>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-3 border-t border-zinc-100 p-4 md:flex-row md:items-end md:justify-between">
        <div className="grid gap-3 sm:grid-cols-2">
          <SelectField label={t("admin.state")} value={stateFilter} onChange={(event) => {
            setStateFilter(event.target.value as DeliveryState | "");
            setOffset(0);
          }}>
            <option value="">{t("admin.allStates")}</option>
            {deliveryStates.map((state) => (
              <option key={state.id} value={state.id}>
                {state.label}
              </option>
            ))}
          </SelectField>
          <SelectField
            label={t("admin.failure")}
            value={failureKind}
            onChange={(event) => {
              setFailureKind(event.target.value as DeliveryFailureKind | "");
              setOffset(0);
            }}
          >
            <option value="">{t("admin.allFailures")}</option>
            {deliveryFailureKinds.map((kind) => (
              <option key={kind.id} value={kind.id}>
                {kind.label}
              </option>
            ))}
          </SelectField>
        </div>
        <Button onClick={() => deliveries.refetch()} disabled={deliveries.isFetching}>
          <RefreshCw size={16} />
          {t("actions.refresh")}
        </Button>
      </div>

      {deliveries.isLoading ? <LoadingState label={t("admin.loadingDeliveries")} /> : null}
      {deliveries.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("admin.deliveriesLoadFailed")} body={errorMessage(deliveries.error, t("admin.deliveryRequestFailed"))} />
        </div>
      ) : null}
      {deliveries.data?.items.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title={t("admin.noDeliveries")} body={t("admin.noDeliveriesBody")} />
        </div>
      ) : null}
      {deliveries.data?.items.map((delivery) => (
        <FederationDeliveryRow
          key={delivery.id}
          delivery={delivery}
          onRetry={(deliveryId) => retryDelivery.mutate(deliveryId)}
          retrying={retryDelivery.isPending}
        />
      ))}
      {deliveries.data ? (
        <div className="border-t border-zinc-100 p-4">
          <OffsetPaginationControls page={deliveries.data} onOffsetChange={setOffset} disabled={deliveries.isFetching} />
        </div>
      ) : null}
    </Panel>
  );
}

export function AdminFederationPage() {
  const { t } = useI18n();
  return (
    <div className="space-y-5">
      <Panel className="p-5">
        <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
          <RadioTower size={14} />
          {t("admin.federationBadge")}
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-950">{t("admin.federationTitle")}</h1>
      </Panel>

      <div className="grid gap-5 2xl:grid-cols-[0.9fr_1.1fr]">
        <DomainBlocksPanel />
        <RemoteActorsPanel />
      </div>
      <FederationDeliveriesPanel />
    </div>
  );
}

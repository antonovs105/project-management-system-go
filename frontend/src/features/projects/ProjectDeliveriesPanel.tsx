import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel, SelectField } from "../../components/ui";
import { OffsetPaginationControls } from "../../components/OffsetPaginationControls";
import { api, errorMessage } from "../../lib/api";
import { deliveryStates } from "../../lib/constants";
import { compactId } from "../../lib/format";
import { useI18n } from "../../lib/i18n-context";
import { queryKeys } from "../../lib/queryKeys";
import type { DeliveryState, ID, ProjectDelivery } from "../../types";
import { useState } from "react";

function stateClass(state: DeliveryState): string {
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

function DeliveryRow({
  delivery,
  onRetry,
  retrying,
}: {
  delivery: ProjectDelivery;
  onRetry: (deliveryId: ID) => void;
  retrying: boolean;
}) {
  const { t, relativeDate } = useI18n();
  return (
    <div className="grid gap-3 border-t border-zinc-100 px-4 py-3 lg:grid-cols-[1.2fr_1fr_0.8fr_auto] lg:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className={`inline-flex rounded-full border px-2 py-0.5 text-xs font-medium ${stateClass(delivery.state)}`}>
            {delivery.state}
          </span>
          <span className="text-sm font-medium text-zinc-950">{delivery.activity_type}</span>
        </div>
        <div className="mt-1 truncate text-xs text-zinc-500">{delivery.activity_ap_id}</div>
      </div>
      <div className="min-w-0">
        <div className="truncate text-sm text-zinc-700">{delivery.target_inbox_url}</div>
        {delivery.target_ap_id ? <div className="mt-1 truncate text-xs text-zinc-500">{delivery.target_ap_id}</div> : null}
      </div>
      <div className="text-sm text-zinc-600">
        <div>
          {t("webhooks.attempts", { attempts: delivery.attempts, max: delivery.max_attempts })}
        </div>
        <div className="mt-1 text-xs text-zinc-500">
          {delivery.last_attempt_at ? t("deliveries.tried", { date: relativeDate(delivery.last_attempt_at) }) : t("deliveries.queuedAt", { date: relativeDate(delivery.created_at) })}
        </div>
        {delivery.last_error ? <div className="mt-1 line-clamp-2 text-xs text-red-700">{delivery.last_error}</div> : null}
      </div>
      <div className="flex items-center justify-between gap-2 lg:justify-end">
        <span className="rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">
          {compactId(delivery.id)}
        </span>
        <Button onClick={() => onRetry(delivery.id)} disabled={!delivery.can_retry || retrying}>
          <RotateCcw size={15} />
          {t("deliveries.retry")}
        </Button>
      </div>
    </div>
  );
}

export function ProjectDeliveriesPanel({ projectId }: { projectId: ID }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [stateFilter, setStateFilter] = useState<DeliveryState | "">("");
  const [offset, setOffset] = useState(0);

  const summary = useQuery({
    queryKey: queryKeys.projectDeliverySummary(projectId),
    queryFn: () => api.getProjectDeliverySummary(projectId),
  });

  const deliveries = useQuery({
    queryKey: queryKeys.projectDeliveries(projectId, stateFilter, offset),
    queryFn: () => api.listProjectDeliveriesPage(projectId, { limit: 50, offset }, { state: stateFilter || undefined }),
  });

  const retryDelivery = useMutation({
    mutationFn: (deliveryId: ID) => api.retryProjectDelivery(projectId, deliveryId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.projectDeliveries(projectId, stateFilter) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.projectDeliverySummary(projectId) }),
      ]);
      toast.success(t("deliveries.queued"));
    },
    onError: (error) => toast.error(errorMessage(error, t("deliveries.retryFailed"))),
  });

  const summaryItems = summary.data
    ? [
        [t("overview.total"), summary.data.total],
        [t("admin.pending"), summary.data.pending],
        [t("deliveries.processing"), summary.data.processing],
        [t("deliveries.delivered"), summary.data.delivered],
        [t("overview.failed"), summary.data.failed],
        [t("admin.dead"), summary.data.dead],
        [t("overview.retryable"), summary.data.retryable],
      ]
    : [];

  return (
    <div className="space-y-4">
      {summary.isError ? (
        <ErrorState title={t("deliveries.summaryLoadFailed")} body={errorMessage(summary.error, t("deliveries.summaryRequestFailed"))} />
      ) : null}
      {deliveries.isError ? (
        <ErrorState title={t("deliveries.loadFailed")} body={errorMessage(deliveries.error, t("deliveries.requestFailed"))} />
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {summary.isLoading ? <LoadingState label={t("deliveries.loadingSummary")} /> : null}
        {summaryItems.map(([label, value]) => (
          <Panel key={label} className="p-4">
            <div className="text-xs font-medium uppercase tracking-wide text-zinc-400">{label}</div>
            <div className="mt-2 text-2xl font-semibold text-zinc-950">{value}</div>
          </Panel>
        ))}
      </div>

      <Panel className="overflow-hidden">
        <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-end md:justify-between">
          <SelectField
            label={t("deliveries.state")}
            className="w-full md:w-56"
            value={stateFilter}
            onChange={(event) => {
              setStateFilter(event.target.value as DeliveryState | "");
              setOffset(0);
            }}
          >
            <option value="">{t("deliveries.allStates")}</option>
            {deliveryStates.map((state) => (
              <option key={state.id} value={state.id}>
                {state.label}
              </option>
            ))}
          </SelectField>
          <Button onClick={() => deliveries.refetch()} disabled={deliveries.isFetching}>
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
        </div>

        {deliveries.isLoading ? <LoadingState label={t("deliveries.loading")} /> : null}
        {deliveries.data?.items.length === 0 ? (
          <div className="border-t border-zinc-100 px-4 py-4">
            <EmptyState title={t("deliveries.empty")} body={t("deliveries.emptyBody")} />
          </div>
        ) : null}
        {deliveries.data?.items.map((delivery) => (
          <DeliveryRow
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
    </div>
  );
}

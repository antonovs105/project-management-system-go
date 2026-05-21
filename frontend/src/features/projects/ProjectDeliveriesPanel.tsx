import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { Button, EmptyState, ErrorState, LoadingState, Panel, SelectField } from "../../components/ui";
import { api, errorMessage } from "../../lib/api";
import { deliveryStates } from "../../lib/constants";
import { compactId, relativeDate } from "../../lib/format";
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
          {delivery.attempts}/{delivery.max_attempts} attempts
        </div>
        <div className="mt-1 text-xs text-zinc-500">
          {delivery.last_attempt_at ? `Tried ${relativeDate(delivery.last_attempt_at)}` : `Queued ${relativeDate(delivery.created_at)}`}
        </div>
        {delivery.last_error ? <div className="mt-1 line-clamp-2 text-xs text-red-700">{delivery.last_error}</div> : null}
      </div>
      <div className="flex items-center justify-between gap-2 lg:justify-end">
        <span className="rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">
          {compactId(delivery.id)}
        </span>
        <Button onClick={() => onRetry(delivery.id)} disabled={!delivery.can_retry || retrying}>
          <RotateCcw size={15} />
          Retry
        </Button>
      </div>
    </div>
  );
}

export function ProjectDeliveriesPanel({ projectId }: { projectId: ID }) {
  const queryClient = useQueryClient();
  const [stateFilter, setStateFilter] = useState<DeliveryState | "">("");

  const summary = useQuery({
    queryKey: queryKeys.projectDeliverySummary(projectId),
    queryFn: () => api.getProjectDeliverySummary(projectId),
  });

  const deliveries = useQuery({
    queryKey: queryKeys.projectDeliveries(projectId, stateFilter),
    queryFn: () => api.listProjectDeliveries(projectId, { state: stateFilter || undefined }),
  });

  const retryDelivery = useMutation({
    mutationFn: (deliveryId: ID) => api.retryProjectDelivery(projectId, deliveryId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.projectDeliveries(projectId, stateFilter) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.projectDeliverySummary(projectId) }),
      ]);
      toast.success("Delivery queued");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not retry delivery.")),
  });

  const summaryItems = summary.data
    ? [
        ["Total", summary.data.total],
        ["Pending", summary.data.pending],
        ["Processing", summary.data.processing],
        ["Delivered", summary.data.delivered],
        ["Failed", summary.data.failed],
        ["Dead", summary.data.dead],
        ["Retryable", summary.data.retryable],
      ]
    : [];

  return (
    <div className="space-y-4">
      {summary.isError ? (
        <ErrorState title="Could not load delivery summary" body={errorMessage(summary.error, "Delivery summary request failed.")} />
      ) : null}
      {deliveries.isError ? (
        <ErrorState title="Could not load deliveries" body={errorMessage(deliveries.error, "Delivery list request failed.")} />
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {summary.isLoading ? <LoadingState label="Loading delivery summary" /> : null}
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
            label="State"
            className="w-full md:w-56"
            value={stateFilter}
            onChange={(event) => setStateFilter(event.target.value as DeliveryState | "")}
          >
            <option value="">All states</option>
            {deliveryStates.map((state) => (
              <option key={state.id} value={state.id}>
                {state.label}
              </option>
            ))}
          </SelectField>
          <Button onClick={() => deliveries.refetch()} disabled={deliveries.isFetching}>
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>

        {deliveries.isLoading ? <LoadingState label="Loading deliveries" /> : null}
        {deliveries.data?.length === 0 ? (
          <div className="border-t border-zinc-100 px-4 py-4">
            <EmptyState title="No deliveries" body="No outbound delivery records match the current filter." />
          </div>
        ) : null}
        {deliveries.data?.map((delivery) => (
          <DeliveryRow
            key={delivery.id}
            delivery={delivery}
            onRetry={(deliveryId) => retryDelivery.mutate(deliveryId)}
            retrying={retryDelivery.isPending}
          />
        ))}
      </Panel>
    </div>
  );
}

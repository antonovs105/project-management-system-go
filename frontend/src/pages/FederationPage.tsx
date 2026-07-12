import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, CheckCircle2, Clock3, Inbox, Loader2, RadioTower, RefreshCw, Search, UserPlus, XCircle } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Badge, Button, EmptyState, ErrorState, LoadingState, Panel, SelectField, TextField } from "../components/ui";
import { OffsetPaginationControls } from "../components/OffsetPaginationControls";
import { api, errorMessage } from "../lib/api";
import { compactId, relativeDate } from "../lib/format";
import { useI18n, type I18nContextValue } from "../lib/i18n-context";
import { queryKeys } from "../lib/queryKeys";
import type { FederationFollowState, FederationInboxActivity, FederationRemoteActor, FederationRemoteFollow, FollowRemoteActorResult } from "../types";

type Translator = I18nContextValue["t"];
const federationPageSize = 25;

function followStateOptions(t: Translator): Array<{ id: FederationFollowState | ""; label: string }> {
  return [
    { id: "", label: t("federation.allStates") },
    { id: "accepted", label: t("federation.accepted") },
    { id: "pending", label: t("federation.pending") },
    { id: "rejected", label: t("federation.rejected") },
  ];
}

function followStateLabel(state: FederationFollowState, t: Translator): string {
  switch (state) {
    case "accepted":
      return t("federation.accepted");
    case "rejected":
      return t("federation.rejected");
    default:
      return t("federation.pending");
  }
}

function followStateClass(state: FederationFollowState): string {
  switch (state) {
    case "accepted":
      return "border-zinc-950 bg-zinc-950 text-white";
    case "rejected":
      return "border-red-200 bg-red-50 text-red-700";
    default:
      return "border-zinc-200 bg-zinc-50 text-zinc-500";
  }
}

function followIcon(state: FederationFollowState) {
  switch (state) {
    case "accepted":
      return <CheckCircle2 size={15} />;
    case "rejected":
      return <XCircle size={15} />;
    default:
      return <Clock3 size={15} />;
  }
}

type ActorLabelFields = {
  name?: string;
  handle?: string;
  preferred_username?: string;
  ap_id?: string;
  actor_ap_id?: string;
  type?: string;
  actor_type?: string;
};

function actorLabel(actor: ActorLabelFields, t: Translator): string {
  return actor.name || actor.handle || actor.preferred_username || actor.ap_id || actor.actor_ap_id || t("federation.remoteActorFallback");
}

function actorAPID(actor: ActorLabelFields): string {
  return actor.ap_id || actor.actor_ap_id || "";
}

function actorType(actor: ActorLabelFields, t: Translator): string {
  return actor.type || actor.actor_type || t("federation.actorFallback");
}

function activityObjectLabel(activity: FederationInboxActivity): string {
  return activity.object_name || activity.object_ap_id || activity.activity_ap_id;
}

function ActivityRow({ activity }: { activity: FederationInboxActivity }) {
  const { t } = useI18n();
  const target = activity.target_name || activity.target_handle || activity.target_ap_id;

  return (
    <div className="grid gap-3 border-t border-zinc-100 px-4 py-3 lg:grid-cols-[1.2fr_1fr_auto] lg:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{activity.activity_type}</Badge>
          {activity.object_type ? <Badge className="border-zinc-200 bg-white text-zinc-600">{activity.object_type}</Badge> : null}
        </div>
        <div className="mt-2 truncate text-sm font-medium text-zinc-950">{activityObjectLabel(activity)}</div>
        {activity.object_content ? <div className="mt-1 line-clamp-2 text-sm text-zinc-500">{activity.object_content}</div> : null}
      </div>

      <div className="min-w-0 text-sm text-zinc-600">
        <div className="truncate">{activity.actor_name || activity.actor_handle || activity.actor_ap_id}</div>
        {target ? <div className="mt-1 truncate text-xs text-zinc-400">{target}</div> : null}
      </div>

      <div className="flex items-center justify-between gap-2 lg:justify-end">
        <span className="rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">
          {relativeDate(activity.received_at)}
        </span>
        {activity.object_ap_id ? (
          <a
            href={activity.object_ap_id}
            target="_blank"
            rel="noreferrer"
            className="focus-ring inline-flex h-8 w-8 items-center justify-center rounded-full border border-zinc-200 text-zinc-500 transition hover:bg-zinc-50 hover:text-zinc-950"
            title={t("federation.openObject")}
            aria-label={t("federation.openObject")}
          >
            <ArrowUpRight size={15} />
          </a>
        ) : null}
      </div>
    </div>
  );
}

function FollowRow({ follow }: { follow: FederationRemoteFollow }) {
  const { t } = useI18n();
  return (
    <div className="grid gap-3 border-t border-zinc-100 px-4 py-3 lg:grid-cols-[1fr_1.2fr_auto] lg:items-center">
      <div className="min-w-0">
        <div className="font-medium text-zinc-950">{actorLabel(follow, t)}</div>
        <div className="mt-1 text-xs text-zinc-500">{follow.actor_type}</div>
      </div>
      <div className="min-w-0 text-sm text-zinc-600">
        <div className="truncate">{follow.actor_ap_id}</div>
        <div className="mt-1 truncate text-xs text-zinc-400">{follow.inbox_url}</div>
      </div>
      <div className="flex items-center justify-between gap-2 lg:justify-end">
        <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${followStateClass(follow.state)}`}>
          {followIcon(follow.state)}
          {followStateLabel(follow.state, t)}
        </span>
        <span className="rounded-full border border-zinc-200 px-2 py-0.5 text-xs text-zinc-500">
          {compactId(follow.actor_id)}
        </span>
      </div>
    </div>
  );
}

function ResolvedActorCard({
  actor,
  onFollow,
  following,
}: {
  actor: FederationRemoteActor;
  onFollow: (actor: FederationRemoteActor) => void;
  following: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="rounded-2xl border border-zinc-200 bg-white p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{actorType(actor, t)}</Badge>
            {actor.handle ? <Badge className="border-zinc-950 bg-zinc-950 text-white">{actor.handle}</Badge> : null}
          </div>
          <h3 className="mt-3 truncate text-base font-semibold text-zinc-950">{actorLabel(actor, t)}</h3>
          <p className="mt-1 truncate text-sm text-zinc-500">{actor.ap_id}</p>
          {actor.summary ? <p className="mt-2 line-clamp-2 text-sm text-zinc-600">{actor.summary}</p> : null}
        </div>
        <Button tone="primary" onClick={() => onFollow(actor)} disabled={following}>
          {following ? <Loader2 size={16} className="animate-spin" /> : <UserPlus size={16} />}
          {t("federation.follow")}
        </Button>
      </div>
    </div>
  );
}

function FollowResultCard({ result }: { result: FollowRemoteActorResult }) {
  const { t } = useI18n();
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50 p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${followStateClass(result.follow.state)}`}>
              {followIcon(result.follow.state)}
              {followStateLabel(result.follow.state, t)}
            </span>
            <Badge className="border-zinc-200 bg-white text-zinc-500">
              {result.created ? t("federation.queued") : t("federation.existing")}
            </Badge>
            {result.delivery ? <Badge className="border-zinc-200 bg-white text-zinc-500">{result.delivery.state}</Badge> : null}
          </div>
          <h3 className="mt-3 truncate text-base font-semibold text-zinc-950">{actorLabel(result.follow, t)}</h3>
          <p className="mt-1 truncate text-sm text-zinc-500">{actorAPID(result.follow)}</p>
        </div>
        <span className="rounded-full border border-zinc-200 bg-white px-2 py-0.5 text-xs text-zinc-500">{compactId(result.follow.actor_id)}</span>
      </div>
    </div>
  );
}

function FederationActionPanel() {
  const queryClient = useQueryClient();
  const { t } = useI18n();
  const [resource, setResource] = useState("");
  const [resolvedActor, setResolvedActor] = useState<FederationRemoteActor | null>(null);
  const [followResult, setFollowResult] = useState<FollowRemoteActorResult | null>(null);
  const trimmedResource = resource.trim();

  const discoverActor = useMutation({
    mutationFn: (target: string) => api.discoverPersonalFederationActor(target),
    onSuccess: (actor) => {
      setResolvedActor(actor);
      setFollowResult(null);
      toast.success(t("federation.resolved"));
    },
    onError: (error) => toast.error(errorMessage(error, t("federation.resolveFailed"))),
  });

  const followActor = useMutation({
    mutationFn: (target: string) => api.followPersonalFederationActor(target),
    onSuccess: (result) => {
      setFollowResult(result);
      queryClient.invalidateQueries({ queryKey: ["personalFederationFollows"] });
      toast.success(result.created ? t("federation.followQueued") : t("federation.alreadyFollowing"));
    },
    onError: (error) => toast.error(errorMessage(error, t("federation.followFailed"))),
  });

  function submitDiscover(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!trimmedResource) {
      toast.error(t("federation.enterRemoteActor"));
      return;
    }
    discoverActor.mutate(trimmedResource);
  }

  function followResolvedActor(actor: FederationRemoteActor) {
    followActor.mutate(actor.ap_id || trimmedResource);
  }

  return (
    <Panel className="p-5">
      <div className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr] xl:items-start">
        <form onSubmit={submitDiscover} className="grid gap-3">
          <TextField
            label={t("federation.remoteActor")}
            value={resource}
            onChange={(event) => setResource(event.target.value)}
            placeholder={t("federation.remoteActorPlaceholder")}
            autoComplete="off"
          />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" tone="primary" disabled={discoverActor.isPending}>
              {discoverActor.isPending ? <Loader2 size={16} className="animate-spin" /> : <Search size={16} />}
              {t("federation.discover")}
            </Button>
            <Button
              onClick={() => {
                if (!trimmedResource) {
                  toast.error(t("federation.enterRemoteActor"));
                  return;
                }
                followActor.mutate(trimmedResource);
              }}
              disabled={followActor.isPending}
            >
              {followActor.isPending ? <Loader2 size={16} className="animate-spin" /> : <UserPlus size={16} />}
              {t("federation.follow")}
            </Button>
          </div>
        </form>

        <div className="grid gap-3">
          {resolvedActor ? <ResolvedActorCard actor={resolvedActor} onFollow={followResolvedActor} following={followActor.isPending} /> : null}
          {followResult ? <FollowResultCard result={followResult} /> : null}
          {!resolvedActor && !followResult ? (
            <div className="flex min-h-32 items-center justify-center rounded-2xl border border-dashed border-zinc-300 bg-zinc-50 px-4 py-6 text-sm text-zinc-500">
              {t("federation.resolveOrFollow")}
            </div>
          ) : null}
        </div>
      </div>
    </Panel>
  );
}

function FederationInboxPanel() {
  const { t } = useI18n();
  const [offset, setOffset] = useState(0);
  const inbox = useQuery({
    queryKey: [...queryKeys.personalFederationInbox, "page", offset],
    queryFn: () => api.listPersonalFederationInboxPage({ limit: federationPageSize, offset }),
  });

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-center md:justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <Inbox size={17} />
          {t("federation.inbox")}
        </h2>
        <Button onClick={() => inbox.refetch()} disabled={inbox.isFetching}>
          <RefreshCw size={16} />
          {t("actions.refresh")}
        </Button>
      </div>

      {inbox.isLoading ? <LoadingState label={t("federation.loadingInbox")} /> : null}
      {inbox.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("federation.loadInboxFailed")} body={errorMessage(inbox.error, t("federation.inboxRequestFailed"))} />
        </div>
      ) : null}
      {inbox.data?.items.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title={t("federation.emptyInboxTitle")} body={t("federation.emptyInboxBody")} />
        </div>
      ) : null}
      {inbox.data?.items.map((activity) => (
        <ActivityRow key={activity.id} activity={activity} />
      ))}
      {inbox.data ? <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={inbox.data} onOffsetChange={setOffset} disabled={inbox.isFetching} /></div> : null}
    </Panel>
  );
}

function RemoteFollowsPanel() {
  const { t } = useI18n();
  const [state, setState] = useState<FederationFollowState | "">("");
  const [offset, setOffset] = useState(0);
  const follows = useQuery({
    queryKey: [...queryKeys.personalFederationFollows(state), "page", offset],
    queryFn: () => api.listPersonalFederationFollowsPage({ limit: federationPageSize, offset }, { state: state || undefined }),
  });

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-end md:justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <RadioTower size={17} />
          {t("federation.remoteFollows")}
        </h2>
        <div className="flex gap-2">
          <SelectField label={t("federation.state")} value={state} onChange={(event) => { setState(event.target.value as FederationFollowState | ""); setOffset(0); }}>
            {followStateOptions(t).map((option) => (
              <option key={option.id || "all"} value={option.id}>
                {option.label}
              </option>
            ))}
          </SelectField>
          <Button onClick={() => follows.refetch()} disabled={follows.isFetching} className="self-end">
            <RefreshCw size={16} />
            {t("actions.refresh")}
          </Button>
        </div>
      </div>

      {follows.isLoading ? <LoadingState label={t("federation.loadingFollows")} /> : null}
      {follows.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title={t("federation.loadFollowsFailed")} body={errorMessage(follows.error, t("federation.followRequestFailed"))} />
        </div>
      ) : null}
      {follows.data?.items.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title={t("federation.emptyFollowsTitle")} body={t("federation.emptyFollowsBody")} />
        </div>
      ) : null}
      {follows.data?.items.map((follow) => (
        <FollowRow key={follow.actor_id} follow={follow} />
      ))}
      {follows.data ? <div className="border-t border-zinc-100 p-4"><OffsetPaginationControls page={follows.data} onOffsetChange={setOffset} disabled={follows.isFetching} /></div> : null}
    </Panel>
  );
}

export function FederationPage() {
  const { t } = useI18n();

  return (
    <div className="space-y-5">
      <Panel className="p-5">
        <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
          <RadioTower size={14} />
          {t("federation.title")}
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-950">{t("federation.title")}</h1>
      </Panel>

      <FederationActionPanel />

      <div className="grid gap-5 2xl:grid-cols-[0.85fr_1.15fr]">
        <RemoteFollowsPanel />
        <FederationInboxPanel />
      </div>
    </div>
  );
}

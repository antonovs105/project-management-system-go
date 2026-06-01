import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, CheckCircle2, Clock3, Inbox, Loader2, RadioTower, RefreshCw, Search, UserPlus, XCircle } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Badge, Button, EmptyState, ErrorState, LoadingState, Panel, SelectField, TextField } from "../components/ui";
import { api, errorMessage } from "../lib/api";
import { compactId, relativeDate } from "../lib/format";
import { queryKeys } from "../lib/queryKeys";
import type { FederationFollowState, FederationInboxActivity, FederationRemoteActor, FederationRemoteFollow, FollowRemoteActorResult } from "../types";

const followStateOptions: Array<{ id: FederationFollowState | ""; label: string }> = [
  { id: "", label: "All states" },
  { id: "accepted", label: "Accepted" },
  { id: "pending", label: "Pending" },
  { id: "rejected", label: "Rejected" },
];

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

function actorLabel(actor: ActorLabelFields): string {
  return actor.name || actor.handle || actor.preferred_username || actor.ap_id || actor.actor_ap_id || "Remote actor";
}

function actorAPID(actor: ActorLabelFields): string {
  return actor.ap_id || actor.actor_ap_id || "";
}

function actorType(actor: ActorLabelFields): string {
  return actor.type || actor.actor_type || "Actor";
}

function activityObjectLabel(activity: FederationInboxActivity): string {
  return activity.object_name || activity.object_ap_id || activity.activity_ap_id;
}

function ActivityRow({ activity }: { activity: FederationInboxActivity }) {
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
            title="Open ActivityPub object"
            aria-label="Open ActivityPub object"
          >
            <ArrowUpRight size={15} />
          </a>
        ) : null}
      </div>
    </div>
  );
}

function FollowRow({ follow }: { follow: FederationRemoteFollow }) {
  return (
    <div className="grid gap-3 border-t border-zinc-100 px-4 py-3 lg:grid-cols-[1fr_1.2fr_auto] lg:items-center">
      <div className="min-w-0">
        <div className="font-medium text-zinc-950">{actorLabel(follow)}</div>
        <div className="mt-1 text-xs text-zinc-500">{follow.actor_type}</div>
      </div>
      <div className="min-w-0 text-sm text-zinc-600">
        <div className="truncate">{follow.actor_ap_id}</div>
        <div className="mt-1 truncate text-xs text-zinc-400">{follow.inbox_url}</div>
      </div>
      <div className="flex items-center justify-between gap-2 lg:justify-end">
        <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${followStateClass(follow.state)}`}>
          {followIcon(follow.state)}
          {follow.state}
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
  return (
    <div className="rounded-2xl border border-zinc-200 bg-white p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge className="border-zinc-200 bg-zinc-50 text-zinc-500">{actorType(actor)}</Badge>
            {actor.handle ? <Badge className="border-zinc-950 bg-zinc-950 text-white">{actor.handle}</Badge> : null}
          </div>
          <h3 className="mt-3 truncate text-base font-semibold text-zinc-950">{actorLabel(actor)}</h3>
          <p className="mt-1 truncate text-sm text-zinc-500">{actor.ap_id}</p>
          {actor.summary ? <p className="mt-2 line-clamp-2 text-sm text-zinc-600">{actor.summary}</p> : null}
        </div>
        <Button tone="primary" onClick={() => onFollow(actor)} disabled={following}>
          {following ? <Loader2 size={16} className="animate-spin" /> : <UserPlus size={16} />}
          Follow
        </Button>
      </div>
    </div>
  );
}

function FollowResultCard({ result }: { result: FollowRemoteActorResult }) {
  return (
    <div className="rounded-2xl border border-zinc-200 bg-zinc-50 p-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${followStateClass(result.follow.state)}`}>
              {followIcon(result.follow.state)}
              {result.follow.state}
            </span>
            <Badge className="border-zinc-200 bg-white text-zinc-500">{result.created ? "queued" : "existing"}</Badge>
            {result.delivery ? <Badge className="border-zinc-200 bg-white text-zinc-500">{result.delivery.state}</Badge> : null}
          </div>
          <h3 className="mt-3 truncate text-base font-semibold text-zinc-950">{actorLabel(result.follow)}</h3>
          <p className="mt-1 truncate text-sm text-zinc-500">{actorAPID(result.follow)}</p>
        </div>
        <span className="rounded-full border border-zinc-200 bg-white px-2 py-0.5 text-xs text-zinc-500">{compactId(result.follow.actor_id)}</span>
      </div>
    </div>
  );
}

function FederationActionPanel() {
  const queryClient = useQueryClient();
  const [resource, setResource] = useState("");
  const [resolvedActor, setResolvedActor] = useState<FederationRemoteActor | null>(null);
  const [followResult, setFollowResult] = useState<FollowRemoteActorResult | null>(null);
  const trimmedResource = resource.trim();

  const discoverActor = useMutation({
    mutationFn: (target: string) => api.discoverPersonalFederationActor(target),
    onSuccess: (actor) => {
      setResolvedActor(actor);
      setFollowResult(null);
      toast.success("Remote actor resolved");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not resolve remote actor.")),
  });

  const followActor = useMutation({
    mutationFn: (target: string) => api.followPersonalFederationActor(target),
    onSuccess: (result) => {
      setFollowResult(result);
      queryClient.invalidateQueries({ queryKey: ["personalFederationFollows"] });
      toast.success(result.created ? "Follow queued" : "Already following");
    },
    onError: (error) => toast.error(errorMessage(error, "Could not follow remote actor.")),
  });

  function submitDiscover(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!trimmedResource) {
      toast.error("Enter a remote actor");
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
            label="Remote actor"
            value={resource}
            onChange={(event) => setResource(event.target.value)}
            placeholder="acct:project@remote.test or https://remote.test/projects/id"
            autoComplete="off"
          />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" tone="primary" disabled={discoverActor.isPending}>
              {discoverActor.isPending ? <Loader2 size={16} className="animate-spin" /> : <Search size={16} />}
              Discover
            </Button>
            <Button
              onClick={() => {
                if (!trimmedResource) {
                  toast.error("Enter a remote actor");
                  return;
                }
                followActor.mutate(trimmedResource);
              }}
              disabled={followActor.isPending}
            >
              {followActor.isPending ? <Loader2 size={16} className="animate-spin" /> : <UserPlus size={16} />}
              Follow
            </Button>
          </div>
        </form>

        <div className="grid gap-3">
          {resolvedActor ? <ResolvedActorCard actor={resolvedActor} onFollow={followResolvedActor} following={followActor.isPending} /> : null}
          {followResult ? <FollowResultCard result={followResult} /> : null}
          {!resolvedActor && !followResult ? (
            <div className="flex min-h-32 items-center justify-center rounded-2xl border border-dashed border-zinc-300 bg-zinc-50 px-4 py-6 text-sm text-zinc-500">
              Resolve or follow a remote project actor.
            </div>
          ) : null}
        </div>
      </div>
    </Panel>
  );
}

function FederationInboxPanel() {
  const inbox = useQuery({
    queryKey: queryKeys.personalFederationInbox,
    queryFn: () => api.listPersonalFederationInbox(),
  });

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-center md:justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <Inbox size={17} />
          Inbox
        </h2>
        <Button onClick={() => inbox.refetch()} disabled={inbox.isFetching}>
          <RefreshCw size={16} />
          Refresh
        </Button>
      </div>

      {inbox.isLoading ? <LoadingState label="Loading federation inbox" /> : null}
      {inbox.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not load federation inbox" body={errorMessage(inbox.error, "Inbox request failed.")} />
        </div>
      ) : null}
      {inbox.data?.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title="No federation inbox activity" body="Remote activity delivered to this account will appear here." />
        </div>
      ) : null}
      {inbox.data?.map((activity) => (
        <ActivityRow key={activity.id} activity={activity} />
      ))}
    </Panel>
  );
}

function RemoteFollowsPanel() {
  const [state, setState] = useState<FederationFollowState | "">("");
  const follows = useQuery({
    queryKey: queryKeys.personalFederationFollows(state),
    queryFn: () => api.listPersonalFederationFollows({ state: state || undefined }),
  });

  return (
    <Panel className="overflow-hidden">
      <div className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-end md:justify-between">
        <h2 className="flex items-center gap-2 text-base font-semibold text-zinc-950">
          <RadioTower size={17} />
          Remote Follows
        </h2>
        <div className="flex gap-2">
          <SelectField label="State" value={state} onChange={(event) => setState(event.target.value as FederationFollowState | "")}>
            {followStateOptions.map((option) => (
              <option key={option.id || "all"} value={option.id}>
                {option.label}
              </option>
            ))}
          </SelectField>
          <Button onClick={() => follows.refetch()} disabled={follows.isFetching} className="self-end">
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
      </div>

      {follows.isLoading ? <LoadingState label="Loading remote follows" /> : null}
      {follows.isError ? (
        <div className="border-t border-zinc-100 p-4">
          <ErrorState title="Could not load remote follows" body={errorMessage(follows.error, "Follow request failed.")} />
        </div>
      ) : null}
      {follows.data?.length === 0 ? (
        <div className="border-t border-zinc-100 p-4">
          <EmptyState title="No remote follows" body="Remote actors followed by this account will appear here." />
        </div>
      ) : null}
      {follows.data?.map((follow) => (
        <FollowRow key={follow.actor_id} follow={follow} />
      ))}
    </Panel>
  );
}

export function FederationPage() {
  return (
    <div className="space-y-5">
      <Panel className="p-5">
        <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-zinc-200 bg-zinc-50 px-2.5 py-1 text-xs font-medium text-zinc-500">
          <RadioTower size={14} />
          Federation
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-950">Federation</h1>
      </Panel>

      <FederationActionPanel />

      <div className="grid gap-5 2xl:grid-cols-[0.85fr_1.15fr]">
        <RemoteFollowsPanel />
        <FederationInboxPanel />
      </div>
    </div>
  );
}

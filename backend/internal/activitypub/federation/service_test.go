package federation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serviceRepo struct {
	inboxOptions      ListOptions
	followOptions     ListOptions
	localActor        *LocalActor
	storeActorID      string
	storeRemoteID     string
	storeActivityID   string
	storeActivityAPID string
	storeRemoteAPID   string
	storeDocument     []byte
	storeCreated      bool
	storedFollow      *RemoteFollow
}

func (r *serviceRepo) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	r.inboxOptions = options
	return []InboxActivity{{ID: "activity-1"}}, nil
}

func (r *serviceRepo) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	r.followOptions = options
	return []RemoteFollow{{ActorID: "actor-1", State: options.State}}, nil
}

func (r *serviceRepo) LocalUserActor(ctx context.Context, userID string) (*LocalActor, error) {
	if r.localActor != nil {
		return r.localActor, nil
	}
	return &LocalActor{ID: "user-1", APID: "http://local.test/users/alice"}, nil
}

func (r *serviceRepo) StoreOutgoingFollow(ctx context.Context, actorID, remoteActorID, activityID, activityAPID, remoteActorAPID string, document []byte) (*RemoteFollow, bool, error) {
	r.storeActorID = actorID
	r.storeRemoteID = remoteActorID
	r.storeActivityID = activityID
	r.storeActivityAPID = activityAPID
	r.storeRemoteAPID = remoteActorAPID
	r.storeDocument = append([]byte(nil), document...)
	if r.storedFollow != nil {
		return r.storedFollow, r.storeCreated, nil
	}
	return &RemoteFollow{
		ActorID:           remoteActorID,
		ActorAPID:         remoteActorAPID,
		ActorType:         "Group",
		PreferredUsername: "project-1",
		Handle:            "project-1@remote.test",
		Name:              "Remote Project",
		InboxURL:          "https://remote.test/projects/project-1/inbox",
		OutboxURL:         "https://remote.test/projects/project-1/outbox",
		State:             "pending",
	}, r.storeCreated, nil
}

type fakeResolver struct {
	discoveredResource string
	fetchedURL         string
	actor              *remoteactor.Actor
	err                error
}

func (r *fakeResolver) Discover(ctx context.Context, resource string) (*remoteactor.Actor, error) {
	r.discoveredResource = resource
	if r.err != nil {
		return nil, r.err
	}
	return r.actor, nil
}

func (r *fakeResolver) Fetch(ctx context.Context, actorURL string) (*remoteactor.Actor, error) {
	r.fetchedURL = actorURL
	if r.err != nil {
		return nil, r.err
	}
	return r.actor, nil
}

type fakeDelivery struct {
	activityID     string
	targetInboxURL string
	delivery       *delivery.Delivery
}

func (d *fakeDelivery) Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error) {
	d.activityID = activityID
	d.targetInboxURL = targetInboxURL
	if d.delivery != nil {
		return d.delivery, nil
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &delivery.Delivery{
		ID:             "delivery-1",
		ActivityAPID:   "http://local.test/activities/activity-1",
		TargetInboxURL: targetInboxURL,
		State:          delivery.StatePending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func TestServiceNormalizesPersonalFederationListLimits(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	activities, err := service.ListInboxActivities(context.Background(), "user-1", ListOptions{Limit: 999, Offset: 2})

	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, maxListLimit, repo.inboxOptions.Limit)
	assert.Equal(t, 2, repo.inboxOptions.Offset)
}

func TestServiceFiltersRemoteFollowsByState(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	follows, err := service.ListRemoteFollows(context.Background(), "user-1", ListOptions{State: "accepted"})

	require.NoError(t, err)
	require.Len(t, follows, 1)
	assert.Equal(t, "accepted", repo.followOptions.State)
	assert.Equal(t, defaultListLimit, repo.followOptions.Limit)
}

func TestServiceRejectsInvalidPersonalFederationFilters(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	_, err := service.ListInboxActivities(context.Background(), "user-1", ListOptions{Offset: -1})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListInboxActivities(context.Background(), "user-1", ListOptions{State: "accepted"})
	require.ErrorIs(t, err, ErrInvalidFilter)

	_, err = service.ListRemoteFollows(context.Background(), "user-1", ListOptions{State: "blocked"})
	require.ErrorIs(t, err, ErrInvalidFilter)
}

func TestServiceDiscoversBareRemoteHandle(t *testing.T) {
	resolver := &fakeResolver{actor: testRemoteActor()}
	service := NewService(&serviceRepo{}, WithRemoteActorResolver(resolver))

	actor, err := service.DiscoverRemoteActor(context.Background(), "project-1@remote.test")

	require.NoError(t, err)
	assert.Equal(t, "acct:project-1@remote.test", resolver.discoveredResource)
	assert.Equal(t, "https://remote.test/projects/project-1", actor.APID)
	assert.Equal(t, "Group", actor.Type)
}

func TestServiceFollowsRemoteActorAndQueuesDelivery(t *testing.T) {
	repo := &serviceRepo{storeCreated: true}
	resolver := &fakeResolver{actor: testRemoteActor()}
	queued := &fakeDelivery{}
	service := NewService(
		repo,
		WithConfig(activitypub.NewConfig("http://local.test", "local.test")),
		WithRemoteActorResolver(resolver),
		WithDelivery(queued),
	)

	result, err := service.FollowRemoteActor(context.Background(), "user-1", "https://remote.test/projects/project-1")

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Equal(t, "https://remote.test/projects/project-1", resolver.fetchedURL)
	assert.Equal(t, "user-1", repo.storeActorID)
	assert.Equal(t, "remote-project", repo.storeRemoteID)
	assert.Equal(t, "https://remote.test/projects/project-1", repo.storeRemoteAPID)
	assert.Equal(t, repo.storeActivityID, queued.activityID)
	assert.Equal(t, "https://remote.test/projects/project-1/inbox", queued.targetInboxURL)
	require.NotNil(t, result.Delivery)
	assert.Equal(t, delivery.StatePending, result.Delivery.State)

	var document map[string]any
	require.NoError(t, json.Unmarshal(repo.storeDocument, &document))
	assert.Equal(t, "Follow", document["type"])
	assert.Equal(t, "http://local.test/users/alice", document["actor"])
	assert.Equal(t, "https://remote.test/projects/project-1", document["object"])
	assert.Equal(t, repo.storeActivityAPID, document["id"])
}

func TestServiceDoesNotQueueDeliveryForExistingFollow(t *testing.T) {
	repo := &serviceRepo{storeCreated: false}
	resolver := &fakeResolver{actor: testRemoteActor()}
	queued := &fakeDelivery{}
	service := NewService(repo, WithRemoteActorResolver(resolver), WithDelivery(queued))

	result, err := service.FollowRemoteActor(context.Background(), "user-1", "project-1@remote.test")

	require.NoError(t, err)
	assert.False(t, result.Created)
	assert.Nil(t, result.Delivery)
	assert.Empty(t, queued.activityID)
	assert.Equal(t, "pending", result.Follow.State)
}

func TestServiceRejectsInvalidRemoteActorResource(t *testing.T) {
	service := NewService(&serviceRepo{}, WithRemoteActorResolver(&fakeResolver{actor: testRemoteActor()}))

	_, err := service.DiscoverRemoteActor(context.Background(), "project without domain")

	require.ErrorIs(t, err, ErrInvalidRemoteResource)
}

func testRemoteActor() *remoteactor.Actor {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &remoteactor.Actor{
		ID:                "remote-project",
		APID:              "https://remote.test/projects/project-1",
		Type:              "Group",
		PreferredUsername: "project-1",
		Handle:            "project-1@remote.test",
		Name:              "Remote Project",
		Summary:           "Remote",
		InboxURL:          "https://remote.test/projects/project-1/inbox",
		OutboxURL:         "https://remote.test/projects/project-1/outbox",
		LastFetchedAt:     &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serviceRepo struct {
	inboxOptions         ListOptions
	followOptions        ListOptions
	inviteOptions        ListOptions
	projectOptions       ListOptions
	localActor           *LocalActor
	remoteInvite         *RemoteProjectInvite
	remoteProject        *RemoteProject
	storeActorID         string
	storeRemoteID        string
	storeActivityID      string
	storeActivityAPID    string
	storeRemoteAPID      string
	storeDocument        []byte
	storeCreated         bool
	storedFollow         *RemoteFollow
	responseActivityID   string
	responseActivityAPID string
	responseDocument     []byte
	responseStatus       string
	remoteActivityID     string
	remoteActivityAPID   string
	remoteActivityType   string
	remoteActivityObject string
	remoteActivityTarget *string
	remoteActivityDoc    []byte
	remoteObjectDoc      []byte
	remoteObjectDeleted  bool
}

func (r *serviceRepo) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	r.inboxOptions = options
	return []InboxActivity{{ID: "activity-1"}}, nil
}

func (r *serviceRepo) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	r.followOptions = options
	return []RemoteFollow{{ActorID: "actor-1", State: options.State}}, nil
}

func (r *serviceRepo) ListRemoteProjectInvites(ctx context.Context, userID string, options ListOptions) ([]RemoteProjectInvite, error) {
	r.inviteOptions = options
	return []RemoteProjectInvite{{ID: "invite-1", Status: options.State}}, nil
}

func (r *serviceRepo) ListRemoteProjects(ctx context.Context, userID string, options ListOptions) ([]RemoteProject, error) {
	r.projectOptions = options
	return []RemoteProject{*r.remoteProjectValue("remote-project-1")}, nil
}

func (r *serviceRepo) GetRemoteProject(ctx context.Context, userID string, projectID string) (*RemoteProject, error) {
	return r.remoteProjectValue(projectID), nil
}

func (r *serviceRepo) GetRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInvite, error) {
	if r.remoteInvite != nil {
		return r.remoteInvite, nil
	}
	return &RemoteProjectInvite{
		ID:             inviteID,
		InviteAPID:     "https://remote.test/activities/invite-1",
		ProjectAPID:    "https://remote.test/projects/board",
		ProjectName:    "Remote Board",
		TargetInboxURL: "https://remote.test/projects/board/inbox",
		Status:         "pending",
	}, nil
}

func (r *serviceRepo) LocalUserActor(ctx context.Context, userID string) (*LocalActor, error) {
	if r.localActor != nil {
		return r.localActor, nil
	}
	return &LocalActor{ID: "user-1", APID: "http://local.test/users/alice"}, nil
}

func (r *serviceRepo) remoteProjectValue(projectID string) *RemoteProject {
	if r.remoteProject != nil {
		return r.remoteProject
	}
	return &RemoteProject{
		ID:             projectID,
		ProjectAPID:    "https://remote.test/projects/board",
		ProjectName:    "Remote Board",
		Role:           "developer",
		TargetInboxURL: "https://remote.test/projects/board/inbox",
		InviterActorID: "remote-owner-1",
		InviterAPID:    "https://remote.test/users/owner",
		InviterHandle:  "owner@remote.test",
		InviterName:    "Remote Owner",
		CreatedAt:      time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
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

func (r *serviceRepo) AcceptRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error) {
	r.responseActivityID = activityID
	r.responseActivityAPID = activityAPID
	r.responseDocument = append([]byte(nil), document...)
	r.responseStatus = "accepted"
	invite, err := r.GetRemoteProjectInvite(ctx, userID, inviteID)
	if err != nil {
		return nil, err
	}
	copy := *invite
	copy.Status = "accepted"
	return &RemoteInviteResponse{Invite: &copy, ActivityID: activityID, TargetInboxURL: invite.TargetInboxURL}, nil
}

func (r *serviceRepo) RejectRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error) {
	r.responseActivityID = activityID
	r.responseActivityAPID = activityAPID
	r.responseDocument = append([]byte(nil), document...)
	r.responseStatus = "rejected"
	invite, err := r.GetRemoteProjectInvite(ctx, userID, inviteID)
	if err != nil {
		return nil, err
	}
	copy := *invite
	copy.Status = "rejected"
	return &RemoteInviteResponse{Invite: &copy, ActivityID: activityID, TargetInboxURL: invite.TargetInboxURL}, nil
}

func (r *serviceRepo) StoreRemoteProjectActivity(ctx context.Context, userID string, projectID string, activityID string, activityAPID string, activityType string, objectAPID string, targetAPID *string, document []byte, objectDocument []byte, objectDeleted bool) (*RemoteProjectActivity, error) {
	r.remoteActivityID = activityID
	r.remoteActivityAPID = activityAPID
	r.remoteActivityType = activityType
	r.remoteActivityObject = objectAPID
	r.remoteActivityTarget = targetAPID
	r.remoteActivityDoc = append([]byte(nil), document...)
	r.remoteObjectDoc = append([]byte(nil), objectDocument...)
	r.remoteObjectDeleted = objectDeleted
	project := r.remoteProjectValue(projectID)
	return &RemoteProjectActivity{ActivityID: activityID, TargetInboxURL: project.TargetInboxURL}, nil
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

type fakeSigner struct {
	actorIDs []string
	urls     []string
}

func (s *fakeSigner) SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error {
	s.actorIDs = append(s.actorIDs, actorID)
	if req != nil && req.URL != nil {
		s.urls = append(s.urls, req.URL.String())
	}
	req.Header.Set("Signature-Input", `ap=("@method" "@authority" "@path" "date");created=1;keyid="test"`)
	req.Header.Set("Signature", "ap=:dGVzdA==:")
	return nil
}

type fakeHTTPClient struct {
	responses map[string][]byte
	urls      []string
}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.urls = append(c.urls, req.URL.String())
	raw, ok := c.responses[req.URL.String()]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(http.NoBody),
		}, nil
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/activity+json")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(bytesReader(raw)),
	}, nil
}

func bytesReader(raw []byte) *bytes.Reader {
	return bytes.NewReader(raw)
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

func TestServiceFiltersRemoteProjectInvitesByState(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	invites, err := service.ListRemoteProjectInvites(context.Background(), "user-1", ListOptions{State: "pending"})

	require.NoError(t, err)
	require.Len(t, invites, 1)
	assert.Equal(t, "pending", repo.inviteOptions.State)
	assert.Equal(t, defaultListLimit, repo.inviteOptions.Limit)
}

func TestServiceListsAcceptedRemoteProjects(t *testing.T) {
	repo := &serviceRepo{}
	service := NewService(repo)

	projects, err := service.ListRemoteProjects(context.Background(), "user-1", ListOptions{Limit: 25, Offset: 3})

	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "Remote Board", projects[0].ProjectName)
	assert.Equal(t, 25, repo.projectOptions.Limit)
	assert.Equal(t, 3, repo.projectOptions.Offset)
}

func TestServiceListsRemoteProjectTicketsWithSignedReads(t *testing.T) {
	repo := &serviceRepo{}
	signer := &fakeSigner{}
	ticketAPID := "https://remote.test/tickets/ticket-1"
	collectionURL := remoteCollectionPageURL("https://remote.test/projects/board/tickets", defaultListLimit, 0)
	ticketDoc := activitypub.TicketDocument(ticketAPID, "https://remote.test/projects/board", "https://remote.test/users/owner", "Remote ticket", "From remote.", "review", "high", "task", nil, nil, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), false)
	ticketRaw, err := activitypub.MarshalDocument(ticketDoc)
	require.NoError(t, err)
	collectionRaw, err := json.Marshal(map[string]any{
		"id":           collectionURL,
		"type":         "OrderedCollectionPage",
		"orderedItems": []string{ticketAPID},
	})
	require.NoError(t, err)
	client := &fakeHTTPClient{responses: map[string][]byte{
		collectionURL: collectionRaw,
		ticketAPID:    ticketRaw,
	}}
	service := NewService(repo, WithSigner(signer), WithHTTPClient(client))

	tickets, err := service.ListRemoteProjectTickets(context.Background(), "user-1", "remote-project-1", ListOptions{})

	require.NoError(t, err)
	require.Len(t, tickets, 1)
	assert.Equal(t, EncodeRemoteID(ticketAPID), tickets[0].ID)
	assert.Equal(t, "Remote ticket", tickets[0].Title)
	assert.Equal(t, "review", tickets[0].Status)
	assert.Equal(t, []string{collectionURL, ticketAPID}, client.urls)
	assert.Equal(t, []string{"user-1", "user-1"}, signer.actorIDs)
}

func TestServiceListsInlineRemoteProjectTicketsWithoutExtraReads(t *testing.T) {
	repo := &serviceRepo{}
	signer := &fakeSigner{}
	ticketAPID := "https://remote.test/tickets/ticket-1"
	collectionURL := remoteCollectionPageURL("https://remote.test/projects/board/tickets", defaultListLimit, 0)
	ticketDoc := activitypub.TicketDocument(ticketAPID, "https://remote.test/projects/board", "https://remote.test/users/owner", "Inline ticket", "", "open", "medium", "task", nil, nil, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), false)
	collectionRaw, err := json.Marshal(map[string]any{
		"id":           collectionURL,
		"type":         "OrderedCollectionPage",
		"orderedItems": []map[string]any{ticketDoc},
	})
	require.NoError(t, err)
	client := &fakeHTTPClient{responses: map[string][]byte{
		collectionURL: collectionRaw,
	}}
	service := NewService(repo, WithSigner(signer), WithHTTPClient(client))

	tickets, err := service.ListRemoteProjectTickets(context.Background(), "user-1", "remote-project-1", ListOptions{})

	require.NoError(t, err)
	require.Len(t, tickets, 1)
	assert.Equal(t, EncodeRemoteID(ticketAPID), tickets[0].ID)
	assert.Equal(t, "Inline ticket", tickets[0].Title)
	assert.Equal(t, []string{collectionURL}, client.urls)
	assert.Equal(t, []string{"user-1"}, signer.actorIDs)
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

	_, err = service.ListRemoteProjectInvites(context.Background(), "user-1", ListOptions{State: "blocked"})
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

func TestServiceAcceptsRemoteProjectInviteAndQueuesDelivery(t *testing.T) {
	repo := &serviceRepo{}
	resolver := &fakeResolver{actor: testRemoteActor()}
	queued := &fakeDelivery{}
	service := NewService(
		repo,
		WithConfig(activitypub.NewConfig("http://local.test", "local.test")),
		WithRemoteActorResolver(resolver),
		WithDelivery(queued),
	)

	result, err := service.AcceptRemoteProjectInvite(context.Background(), "user-1", "invite-1")

	require.NoError(t, err)
	assert.Equal(t, "https://remote.test/projects/board", resolver.fetchedURL)
	assert.Equal(t, "accepted", repo.responseStatus)
	assert.Equal(t, repo.responseActivityID, queued.activityID)
	assert.Equal(t, "https://remote.test/projects/board/inbox", queued.targetInboxURL)
	assert.Equal(t, "accepted", result.Invite.Status)
	require.NotNil(t, result.Delivery)

	var document map[string]any
	require.NoError(t, json.Unmarshal(repo.responseDocument, &document))
	assert.Equal(t, "Accept", document["type"])
	assert.Equal(t, "http://local.test/users/alice", document["actor"])
	assert.Equal(t, "https://remote.test/activities/invite-1", document["object"])
	assert.Equal(t, "https://remote.test/projects/board", document["target"])
	assert.Equal(t, repo.responseActivityAPID, document["id"])
}

func TestServiceCreatesRemoteTicketAndQueuesDelivery(t *testing.T) {
	repo := &serviceRepo{}
	queued := &fakeDelivery{}
	service := NewService(
		repo,
		WithConfig(activitypub.NewConfig("http://local.test", "local.test")),
		WithDelivery(queued),
	)

	result, err := service.CreateRemoteTicket(context.Background(), "user-1", "remote-project-1", RemoteTicketRequest{
		Title:       "Remote task",
		Description: "Created from the local instance.",
		Priority:    "urgent",
		Type:        "task",
	})

	require.NoError(t, err)
	assert.Equal(t, "Create", repo.remoteActivityType)
	assert.Equal(t, "https://remote.test/projects/board", *repo.remoteActivityTarget)
	assert.Equal(t, repo.remoteActivityID, queued.activityID)
	assert.Equal(t, "https://remote.test/projects/board/inbox", queued.targetInboxURL)
	require.NotNil(t, result.Ticket)
	assert.Equal(t, "Remote task", result.Ticket.Title)
	assert.Equal(t, "urgent", result.Ticket.Priority)
	assert.Equal(t, repo.remoteActivityObject, result.Ticket.APID)

	var document map[string]any
	require.NoError(t, json.Unmarshal(repo.remoteActivityDoc, &document))
	assert.Equal(t, "Create", document["type"])
	assert.Equal(t, "http://local.test/users/alice", document["actor"])
	assert.Equal(t, repo.remoteActivityAPID, document["id"])
	assert.Equal(t, "https://remote.test/projects/board", document["target"])
	object, ok := document["object"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, result.Ticket.APID, object["id"])
	assert.Equal(t, "https://remote.test/projects/board", object["context"])
	assert.Equal(t, "http://local.test/users/alice", object["attributedTo"])

	var storedObject map[string]any
	require.NoError(t, json.Unmarshal(repo.remoteObjectDoc, &storedObject))
	assert.False(t, repo.remoteObjectDeleted)
	assert.Equal(t, result.Ticket.APID, storedObject["id"])
	assert.Equal(t, "Remote task", storedObject["name"])
}

func TestServiceRejectsRemoteTicketCreateWithoutRemoteRolePermission(t *testing.T) {
	repo := &serviceRepo{
		remoteProject: &RemoteProject{
			ID:             "remote-project-1",
			ProjectAPID:    "https://remote.test/projects/board",
			ProjectName:    "Remote Board",
			Role:           "viewer",
			TargetInboxURL: "https://remote.test/projects/board/inbox",
		},
	}
	queued := &fakeDelivery{}
	service := NewService(
		repo,
		WithConfig(activitypub.NewConfig("http://local.test", "local.test")),
		WithDelivery(queued),
	)

	_, err := service.CreateRemoteTicket(context.Background(), "user-1", "remote-project-1", RemoteTicketRequest{
		Title:    "Remote task",
		Priority: "medium",
		Type:     "task",
	})

	require.ErrorIs(t, err, ErrRemoteProjectPermissionDenied)
	assert.Empty(t, repo.remoteActivityID)
	assert.Empty(t, queued.activityID)
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

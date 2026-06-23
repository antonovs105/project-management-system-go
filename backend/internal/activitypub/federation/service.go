package federation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
)

const (
	// defaultListLimit is the fallback personal federation page size.
	defaultListLimit = 100
	// maxListLimit is the largest personal federation page size.
	maxListLimit = 500
	// defaultRemoteRequestTimeout bounds proxied remote workspace reads.
	defaultRemoteRequestTimeout = 15 * time.Second
	// defaultRemoteResponseLimit bounds remote ActivityPub JSON documents.
	defaultRemoteResponseLimit = int64(2 << 20)
	// maxRemoteTicketTitleLength matches local ticket validation for outbound remote writes.
	maxRemoteTicketTitleLength = 120
	// maxRemoteTicketDescriptionLength matches local ticket validation for outbound remote writes.
	maxRemoteTicketDescriptionLength = 4000
)

// Repository defines persistence operations for personal federation views.
type Repository interface {
	ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error)
	ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error)
	ListRemoteProjectInvites(ctx context.Context, userID string, options ListOptions) ([]RemoteProjectInvite, error)
	ListRemoteProjects(ctx context.Context, userID string, options ListOptions) ([]RemoteProject, error)
	GetRemoteProject(ctx context.Context, userID string, projectID string) (*RemoteProject, error)
	GetRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInvite, error)
	LocalUserActor(ctx context.Context, userID string) (*LocalActor, error)
	StoreOutgoingFollow(ctx context.Context, actorID, remoteActorID, activityID, activityAPID, remoteActorAPID string, document []byte) (*RemoteFollow, bool, error)
	AcceptRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error)
	RejectRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error)
	StoreRemoteProjectActivity(ctx context.Context, userID string, projectID string, activityID string, activityAPID string, activityType string, objectAPID string, targetAPID *string, document []byte) (*RemoteProjectActivity, error)
}

// RemoteActorResolver resolves ActivityPub actor resources and URLs.
type RemoteActorResolver interface {
	Discover(ctx context.Context, resource string) (*remoteactor.Actor, error)
	Fetch(ctx context.Context, actorURL string) (*remoteactor.Actor, error)
}

// DeliveryEnqueuer queues outbound ActivityPub delivery work.
type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error)
}

// Signer signs outbound ActivityPub reads for a local actor.
type Signer interface {
	SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error
}

// HTTPClient sends signed remote ActivityPub workspace reads.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Service exposes authenticated personal federation views.
type Service struct {
	repo     Repository
	resolver RemoteActorResolver
	delivery DeliveryEnqueuer
	signer   Signer
	client   HTTPClient
	cfg      activitypub.Config
}

// Option configures the personal federation service.
type Option func(*Service)

// NewService creates a personal federation service.
func NewService(repo Repository, opts ...Option) *Service {
	service := &Service{
		repo:   repo,
		cfg:    activitypub.NewConfig("", ""),
		client: netguard.NewHTTPClient(defaultRemoteRequestTimeout),
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// WithRemoteActorResolver attaches remote actor discovery to the service.
func WithRemoteActorResolver(resolver RemoteActorResolver) Option {
	return func(s *Service) {
		s.resolver = resolver
	}
}

// WithDelivery attaches outbound federation delivery to the service.
func WithDelivery(delivery DeliveryEnqueuer) Option {
	return func(s *Service) {
		s.delivery = delivery
	}
}

// WithSigner attaches the HTTP signature service used for remote workspace reads.
func WithSigner(signer Signer) Option {
	return func(s *Service) {
		s.signer = signer
	}
}

// WithHTTPClient overrides the safe default client used for remote workspace reads.
func WithHTTPClient(client HTTPClient) Option {
	return func(s *Service) {
		if client != nil {
			s.client = client
		}
	}
}

// WithRemoteRequestPolicy configures the safe default HTTP client for remote workspace reads.
func WithRemoteRequestPolicy(requireHTTPS bool, allowPrivateNetworks bool) Option {
	return func(s *Service) {
		policy := make([]netguard.URLPolicyOption, 0, 2)
		if requireHTTPS {
			policy = append(policy, netguard.RequireHTTPS())
		}
		if allowPrivateNetworks {
			policy = append(policy, netguard.AllowPrivateNetworks())
		}
		s.client = netguard.NewHTTPClientWithPolicy(defaultRemoteRequestTimeout, policy...)
	}
}

// WithConfig sets local ActivityPub URL construction.
func WithConfig(cfg activitypub.Config) Option {
	return func(s *Service) {
		s.cfg = cfg
	}
}

// ListInboxActivities returns normalized ActivityPub inbox items for the current user.
func (s *Service) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	options, err := normalizeListOptions(options, false)
	if err != nil {
		return nil, err
	}
	return s.repo.ListInboxActivities(ctx, userID, options)
}

// ListRemoteFollows returns remote actors followed by the current user.
func (s *Service) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	options, err := normalizeListOptions(options, true)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRemoteFollows(ctx, userID, options)
}

// ListRemoteProjectInvites returns remote project invites addressed to the current user.
func (s *Service) ListRemoteProjectInvites(ctx context.Context, userID string, options ListOptions) ([]RemoteProjectInvite, error) {
	options, err := normalizeListOptions(options, true)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRemoteProjectInvites(ctx, userID, options)
}

// ListRemoteProjects returns accepted remote project workspaces for the current user.
func (s *Service) ListRemoteProjects(ctx context.Context, userID string, options ListOptions) ([]RemoteProject, error) {
	options, err := normalizeListOptions(options, false)
	if err != nil {
		return nil, err
	}
	return s.repo.ListRemoteProjects(ctx, userID, options)
}

// GetRemoteProject returns one accepted remote project workspace for the current user.
func (s *Service) GetRemoteProject(ctx context.Context, userID string, projectID string) (*RemoteProject, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, ErrRemoteProjectNotFound
	}
	return s.repo.GetRemoteProject(ctx, userID, projectID)
}

// ListRemoteProjectTickets returns remote project tickets through signed ActivityPub reads.
func (s *Service) ListRemoteProjectTickets(ctx context.Context, userID string, projectID string, options ListOptions) ([]RemoteTicket, error) {
	options, err := normalizeListOptions(options, false)
	if err != nil {
		return nil, err
	}
	project, err := s.GetRemoteProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	collectionURL := remoteCollectionPageURL(activitypub.ProjectTickets(project.ProjectAPID), options.Limit, options.Offset)
	var collection remoteCollectionDocument
	if err := s.getRemoteJSON(ctx, userID, collectionURL, &collection); err != nil {
		return nil, err
	}
	ticketAPIDs := collectionItemIDs(collection.OrderedItems)
	tickets := make([]RemoteTicket, 0, len(ticketAPIDs))
	for _, ticketAPID := range ticketAPIDs {
		remoteTicket, err := s.GetRemoteTicket(ctx, userID, project.ID, EncodeRemoteID(ticketAPID))
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, *remoteTicket)
	}
	return tickets, nil
}

// GetRemoteTicket returns one remote ticket document through a signed ActivityPub read.
func (s *Service) GetRemoteTicket(ctx context.Context, userID string, projectID string, ticketID string) (*RemoteTicket, error) {
	project, err := s.GetRemoteProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	ticketAPID, err := DecodeRemoteID(ticketID)
	if err != nil || ticketAPID == "" {
		return nil, ErrRemoteTicketNotFound
	}
	var doc map[string]any
	raw, err := s.getRemoteRaw(ctx, userID, ticketAPID, &doc)
	if err != nil {
		return nil, err
	}
	return remoteTicketFromDocument(project.ID, project.ProjectAPID, raw, doc)
}

// CreateRemoteTicket queues a signed Create Ticket activity to the remote project inbox.
func (s *Service) CreateRemoteTicket(ctx context.Context, userID string, projectID string, req RemoteTicketRequest) (*RemoteTicketWriteResult, error) {
	project, localActor, err := s.remoteProjectWriteContext(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	title, description, priority, ticketType, err := normalizeRemoteTicketCreate(req)
	if err != nil {
		return nil, err
	}
	ticketUUID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	ticketAPID := activitypub.TicketAPID(s.cfg, ticketUUID)
	now := time.Now().UTC()
	ticketDoc := activitypub.TicketDocument(ticketAPID, project.ProjectAPID, localActor.APID, title, description, "open", priority, ticketType, nil, nil, now, false)
	document, err := s.remoteProjectActivityDocument("Create", localActor.APID, ticketDoc, project.ProjectAPID)
	if err != nil {
		return nil, err
	}
	result, err := s.storeAndEnqueueRemoteProjectActivity(ctx, userID, project.ID, "Create", ticketAPID, &project.ProjectAPID, document)
	if err != nil {
		return nil, err
	}
	ticket, err := remoteTicketFromDocument(project.ID, project.ProjectAPID, mustMarshalDocument(ticketDoc), ticketDoc)
	if err != nil {
		return nil, err
	}
	result.Ticket = ticket
	return result, nil
}

// UpdateRemoteTicket queues a signed Update Ticket activity to the remote project inbox.
func (s *Service) UpdateRemoteTicket(ctx context.Context, userID string, projectID string, ticketID string, req RemoteTicketUpdateRequest) (*RemoteTicketWriteResult, error) {
	project, localActor, err := s.remoteProjectWriteContext(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	ticket, doc, err := s.remoteTicketDocument(ctx, userID, project.ID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := applyRemoteTicketUpdate(doc, req); err != nil {
		return nil, err
	}
	updatedTicket, err := remoteTicketFromDocument(project.ID, project.ProjectAPID, mustMarshalDocument(doc), doc)
	if err != nil {
		return nil, err
	}
	document, err := s.remoteProjectActivityDocument("Update", localActor.APID, doc, project.ProjectAPID)
	if err != nil {
		return nil, err
	}
	result, err := s.storeAndEnqueueRemoteProjectActivity(ctx, userID, project.ID, "Update", ticket.APID, &project.ProjectAPID, document)
	if err != nil {
		return nil, err
	}
	result.Ticket = updatedTicket
	return result, nil
}

// MoveRemoteTicket queues a status-only remote ticket Update activity.
func (s *Service) MoveRemoteTicket(ctx context.Context, userID string, projectID string, ticketID string, req RemoteTicketMoveRequest) (*RemoteTicketWriteResult, error) {
	status := strings.TrimSpace(req.Status)
	resolved := status == "done"
	return s.UpdateRemoteTicket(ctx, userID, projectID, ticketID, RemoteTicketUpdateRequest{
		Status:     &status,
		IsResolved: &resolved,
	})
}

// DeleteRemoteTicket queues a signed Delete activity to the remote project inbox.
func (s *Service) DeleteRemoteTicket(ctx context.Context, userID string, projectID string, ticketID string) (*RemoteTicketWriteResult, error) {
	project, localActor, err := s.remoteProjectWriteContext(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	ticketAPID, err := DecodeRemoteID(ticketID)
	if err != nil || ticketAPID == "" {
		return nil, ErrRemoteTicketNotFound
	}
	document, err := s.remoteProjectActivityDocument("Delete", localActor.APID, ticketAPID, project.ProjectAPID)
	if err != nil {
		return nil, err
	}
	return s.storeAndEnqueueRemoteProjectActivity(ctx, userID, project.ID, "Delete", ticketAPID, &project.ProjectAPID, document)
}

// AcceptRemoteProjectInvite accepts a pending remote project invite and queues an ActivityPub Accept.
func (s *Service) AcceptRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInviteResult, error) {
	if err := s.refreshRemoteInviteProject(ctx, userID, inviteID); err != nil {
		return nil, err
	}
	activityID, activityAPID, document, err := s.remoteInviteResponseDocument(ctx, userID, inviteID, "Accept")
	if err != nil {
		return nil, err
	}
	response, err := s.repo.AcceptRemoteProjectInvite(ctx, userID, inviteID, activityID, activityAPID, document)
	if err != nil {
		return nil, err
	}
	return s.enqueueRemoteInviteResponse(ctx, response)
}

// RejectRemoteProjectInvite rejects a pending remote project invite and queues an ActivityPub Reject.
func (s *Service) RejectRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInviteResult, error) {
	activityID, activityAPID, document, err := s.remoteInviteResponseDocument(ctx, userID, inviteID, "Reject")
	if err != nil {
		return nil, err
	}
	response, err := s.repo.RejectRemoteProjectInvite(ctx, userID, inviteID, activityID, activityAPID, document)
	if err != nil {
		return nil, err
	}
	return s.enqueueRemoteInviteResponse(ctx, response)
}

// remoteInviteResponseDocument builds the signed-actor ActivityPub response body.
func (s *Service) remoteInviteResponseDocument(ctx context.Context, userID string, inviteID string, activityType string) (string, string, []byte, error) {
	localActor, err := s.repo.LocalUserActor(ctx, userID)
	if err != nil {
		return "", "", nil, fmt.Errorf("%w: %v", ErrLocalActorNotFound, err)
	}
	invite, err := s.repo.GetRemoteProjectInvite(ctx, userID, inviteID)
	if err != nil {
		return "", "", nil, err
	}
	if invite.Status != "pending" {
		return "", "", nil, ErrRemoteInviteNotPending
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return "", "", nil, err
	}
	activityAPID := activitypub.ActivityAPID(s.cfg, activityID)
	document, err := activitypub.MarshalDocument(activitypub.ActivityDocument(
		activityType,
		activityAPID,
		localActor.APID,
		invite.InviteAPID,
		invite.ProjectAPID,
		time.Now().UTC(),
	))
	if err != nil {
		return "", "", nil, err
	}
	return activityID, activityAPID, document, nil
}

// refreshRemoteInviteProject best-effort caches the invited remote project actor.
func (s *Service) refreshRemoteInviteProject(ctx context.Context, userID string, inviteID string) error {
	if s.resolver == nil {
		return nil
	}
	invite, err := s.repo.GetRemoteProjectInvite(ctx, userID, inviteID)
	if err != nil {
		return err
	}
	if invite.ProjectAPID == "" || !isHTTPURL(invite.ProjectAPID) {
		return nil
	}
	_, _ = s.resolver.Fetch(ctx, invite.ProjectAPID)
	return nil
}

// enqueueRemoteInviteResponse queues the stored invite response for remote delivery.
func (s *Service) enqueueRemoteInviteResponse(ctx context.Context, response *RemoteInviteResponse) (*RemoteProjectInviteResult, error) {
	if response == nil || response.Invite == nil {
		return nil, ErrRemoteInviteNotFound
	}
	result := &RemoteProjectInviteResult{Invite: *response.Invite}
	if s.delivery != nil && response.ActivityID != "" && response.TargetInboxURL != "" {
		delivery, err := s.delivery.Enqueue(ctx, response.ActivityID, response.TargetInboxURL)
		if err != nil {
			return nil, err
		}
		result.Delivery = followDeliveryProjection(delivery)
	}
	return result, nil
}

// DiscoverRemoteActor resolves and caches a remote actor by acct: resource, handle, or URL.
func (s *Service) DiscoverRemoteActor(ctx context.Context, resource string) (*RemoteActor, error) {
	actor, err := s.resolveRemoteActor(ctx, resource)
	if err != nil {
		return nil, err
	}
	return remoteActorProjection(actor), nil
}

// FollowRemoteActor stores and queues a signed Follow from a local user actor.
func (s *Service) FollowRemoteActor(ctx context.Context, userID string, target string) (*FollowRemoteActorResult, error) {
	localActor, err := s.repo.LocalUserActor(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLocalActorNotFound, err)
	}
	remote, err := s.resolveRemoteActor(ctx, target)
	if err != nil {
		return nil, err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(s.cfg, activityID)
	document, err := activitypub.MarshalDocument(activitypub.ActivityDocument(
		"Follow",
		activityAPID,
		localActor.APID,
		remote.APID,
		nil,
		time.Now().UTC(),
	))
	if err != nil {
		return nil, err
	}

	follow, created, err := s.repo.StoreOutgoingFollow(ctx, localActor.ID, remote.ID, activityID, activityAPID, remote.APID, document)
	if err != nil {
		return nil, err
	}

	var queued *FollowDelivery
	if created && s.delivery != nil {
		delivery, err := s.delivery.Enqueue(ctx, activityID, remote.InboxURL)
		if err != nil {
			return nil, err
		}
		queued = followDeliveryProjection(delivery)
	}

	return &FollowRemoteActorResult{
		Follow:   *follow,
		Delivery: queued,
		Created:  created,
	}, nil
}

// remoteProjectWriteContext loads the remote project and local actor required for outbound work.
func (s *Service) remoteProjectWriteContext(ctx context.Context, userID string, projectID string) (*RemoteProject, *LocalActor, error) {
	project, err := s.GetRemoteProject(ctx, userID, projectID)
	if err != nil {
		return nil, nil, err
	}
	localActor, err := s.repo.LocalUserActor(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrLocalActorNotFound, err)
	}
	return project, localActor, nil
}

// remoteTicketDocument loads a remote ticket and returns both normalized and raw map forms.
func (s *Service) remoteTicketDocument(ctx context.Context, userID string, projectID string, ticketID string) (*RemoteTicket, map[string]any, error) {
	project, err := s.GetRemoteProject(ctx, userID, projectID)
	if err != nil {
		return nil, nil, err
	}
	ticketAPID, err := DecodeRemoteID(ticketID)
	if err != nil || ticketAPID == "" {
		return nil, nil, ErrRemoteTicketNotFound
	}
	var doc map[string]any
	raw, err := s.getRemoteRaw(ctx, userID, ticketAPID, &doc)
	if err != nil {
		return nil, nil, err
	}
	ticket, err := remoteTicketFromDocument(project.ID, project.ProjectAPID, raw, doc)
	if err != nil {
		return nil, nil, err
	}
	return ticket, doc, nil
}

// remoteProjectActivityDocument creates and serializes one outbound project activity.
func (s *Service) remoteProjectActivityDocument(activityType string, actorAPID string, object any, target any) ([]byte, error) {
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(s.cfg, activityID)
	return activitypub.MarshalDocument(activitypub.ActivityDocument(activityType, activityAPID, actorAPID, object, target, time.Now().UTC()))
}

// storeAndEnqueueRemoteProjectActivity stores an outbound activity and queues its delivery.
func (s *Service) storeAndEnqueueRemoteProjectActivity(ctx context.Context, userID string, projectID string, activityType string, objectAPID string, targetAPID *string, document []byte) (*RemoteTicketWriteResult, error) {
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(s.cfg, activityID)
	document, err = setActivityID(document, activityAPID)
	if err != nil {
		return nil, err
	}
	activity, err := s.repo.StoreRemoteProjectActivity(ctx, userID, projectID, activityID, activityAPID, activityType, objectAPID, targetAPID, document)
	if err != nil {
		return nil, err
	}
	result := &RemoteTicketWriteResult{}
	if s.delivery != nil {
		delivery, err := s.delivery.Enqueue(ctx, activity.ActivityID, activity.TargetInboxURL)
		if err != nil {
			return nil, err
		}
		result.Delivery = followDeliveryProjection(delivery)
	}
	return result, nil
}

// setActivityID rewrites the generated activity ID into an already-built document.
func setActivityID(document []byte, activityAPID string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(document, &doc); err != nil {
		return nil, err
	}
	doc["id"] = activityAPID
	return activitypub.MarshalDocument(doc)
}

// getRemoteJSON loads and decodes one signed ActivityPub JSON document.
func (s *Service) getRemoteJSON(ctx context.Context, userID string, remoteURL string, dest any) error {
	_, err := s.getRemoteRaw(ctx, userID, remoteURL, dest)
	return err
}

// getRemoteRaw loads one signed ActivityPub JSON document and returns the raw body.
func (s *Service) getRemoteRaw(ctx context.Context, userID string, remoteURL string, dest any) ([]byte, error) {
	if s.signer == nil || s.client == nil {
		return nil, ErrRemoteRequestFailed
	}
	if !isHTTPURL(remoteURL) {
		return nil, ErrInvalidRemoteResource
	}
	localActor, err := s.repo.LocalUserActor(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLocalActorNotFound, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	req.Header.Set("User-Agent", "project-management-system-go/remote-workspace")
	if err := s.signer.SignRequest(ctx, localActor.ID, req, nil); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteRequestFailed, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRemoteRequestFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRemoteTicketNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: status %d", ErrRemoteRequestFailed, resp.StatusCode)
	}
	if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil && mediaType != "application/activity+json" && mediaType != "application/ld+json" && mediaType != "application/json" {
			return nil, fmt.Errorf("%w: content type %s", ErrRemoteRequestFailed, mediaType)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, defaultRemoteResponseLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > defaultRemoteResponseLimit {
		return nil, fmt.Errorf("%w: response too large", ErrRemoteRequestFailed)
	}
	if dest != nil {
		if err := json.NewDecoder(bytes.NewReader(raw)).Decode(dest); err != nil {
			return nil, fmt.Errorf("%w: invalid json", ErrRemoteRequestFailed)
		}
	}
	return raw, nil
}

// remoteCollectionPageURL builds a paginated ActivityPub collection URL.
func remoteCollectionPageURL(collectionURL string, limit int, offset int) string {
	parsed, err := url.Parse(collectionURL)
	if err != nil {
		return collectionURL
	}
	values := parsed.Query()
	values.Set("page", "true")
	values.Set("limit", fmt.Sprintf("%d", limit))
	values.Set("offset", fmt.Sprintf("%d", offset))
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

// remoteCollectionDocument is the minimal ActivityStreams collection page shape needed for remote tickets.
type remoteCollectionDocument struct {
	OrderedItems []json.RawMessage `json:"orderedItems"`
}

// collectionItemIDs extracts string item IDs from an ActivityStreams collection page.
func collectionItemIDs(items []json.RawMessage) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		var id string
		if err := json.Unmarshal(item, &id); err == nil && strings.TrimSpace(id) != "" {
			ids = append(ids, strings.TrimSpace(id))
			continue
		}
		var object map[string]any
		if err := json.Unmarshal(item, &object); err == nil {
			if id := stringField(object, "id"); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// EncodeRemoteID encodes an ActivityPub ID into a URL-safe local route token.
func EncodeRemoteID(apID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(apID))
}

// DecodeRemoteID decodes a URL-safe local route token into an ActivityPub ID.
func DecodeRemoteID(id string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(id))
	if err != nil {
		return "", err
	}
	value := string(raw)
	if !isHTTPURL(value) {
		return "", ErrInvalidRemoteResource
	}
	return value, nil
}

// remoteTicketFromDocument normalizes a remote ForgeFed ticket document for the UI.
func remoteTicketFromDocument(projectID string, projectAPID string, raw []byte, doc map[string]any) (*RemoteTicket, error) {
	apID := stringField(doc, "id")
	if apID == "" || !isHTTPURL(apID) || !documentTypeContains(doc["type"], "forge:Ticket") {
		return nil, ErrRemoteTicketNotFound
	}
	if contextAPID := stringField(doc, "context"); contextAPID != "" && contextAPID != projectAPID {
		return nil, ErrRemoteTicketNotFound
	}
	reporterAPID := stringField(doc, "attributedTo")
	title := strings.TrimSpace(stringField(doc, "name"))
	if title == "" {
		title = apID
	}
	description := stringField(doc, "content")
	status := normalizeRemoteTicketStatus(stringField(doc, "forge:status"), boolField(doc, "forge:isResolved"))
	priority := normalizeRemoteTicketPriority(stringField(doc, "forge:priority"))
	ticketType := normalizeRemoteTicketType(stringField(doc, "forge:ticketType"))
	createdAt := parseRemoteTime(stringField(doc, "published"))
	updatedAt := parseRemoteTime(stringField(doc, "updated"))
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	parentID := encodedOptionalAPID(stringField(doc, "inReplyTo"))
	assigneeID := encodedOptionalAPID(firstAssignedTo(doc["forge:assignedTo"]))
	return &RemoteTicket{
		ID:          EncodeRemoteID(apID),
		APID:        apID,
		Title:       title,
		Description: description,
		Status:      status,
		Priority:    priority,
		Type:        ticketType,
		Rank:        remoteTicketRank(ticketType),
		ParentID:    parentID,
		ProjectID:   projectID,
		ReporterID:  EncodeRemoteID(reporterAPID),
		AssigneeID:  assigneeID,
		IsResolved:  boolField(doc, "forge:isResolved") || status == "done",
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Raw:         raw,
	}, nil
}

// encodedOptionalAPID returns an encoded remote route token for non-empty HTTP ActivityPub IDs.
func encodedOptionalAPID(apID string) *string {
	if apID == "" || !isHTTPURL(apID) {
		return nil
	}
	encoded := EncodeRemoteID(apID)
	return &encoded
}

// stringField extracts and trims a string field from a decoded JSON object.
func stringField(doc map[string]any, key string) string {
	value, _ := doc[key].(string)
	return strings.TrimSpace(value)
}

// boolField extracts a boolean field from a decoded JSON object.
func boolField(doc map[string]any, key string) bool {
	value, _ := doc[key].(bool)
	return value
}

// documentTypeContains reports whether an ActivityStreams type field contains a value.
func documentTypeContains(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && text == expected {
				return true
			}
		}
	}
	return false
}

// firstAssignedTo extracts the first assigned actor AP ID from a ForgeFed assignedTo field.
func firstAssignedTo(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

// parseRemoteTime parses an ActivityPub timestamp and falls back to the current time.
func parseRemoteTime(raw string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

// normalizeRemoteTicketCreate validates and normalizes outbound remote ticket creation input.
func normalizeRemoteTicketCreate(req RemoteTicketRequest) (string, string, string, string, error) {
	title, err := normalizeRemoteTicketText(req.Title, "title", maxRemoteTicketTitleLength, true)
	if err != nil {
		return "", "", "", "", err
	}
	description, err := normalizeRemoteTicketText(req.Description, "description", maxRemoteTicketDescriptionLength, false)
	if err != nil {
		return "", "", "", "", err
	}
	priority := normalizeRemoteTicketPriority(req.Priority)
	if strings.TrimSpace(req.Priority) != "" && priority != strings.TrimSpace(req.Priority) {
		return "", "", "", "", fmt.Errorf("%w: invalid priority", ErrInvalidRemoteTicketInput)
	}
	ticketType := normalizeRemoteTicketType(req.Type)
	if strings.TrimSpace(req.Type) != "" && ticketType != strings.TrimSpace(req.Type) {
		return "", "", "", "", fmt.Errorf("%w: invalid type", ErrInvalidRemoteTicketInput)
	}
	return title, description, priority, ticketType, nil
}

// applyRemoteTicketUpdate validates and applies outbound remote ticket update input to a ticket document.
func applyRemoteTicketUpdate(doc map[string]any, req RemoteTicketUpdateRequest) error {
	changed := false
	if req.Title != nil {
		value, err := normalizeRemoteTicketText(*req.Title, "title", maxRemoteTicketTitleLength, true)
		if err != nil {
			return err
		}
		doc["name"] = value
		changed = true
	}
	if req.Description != nil {
		value, err := normalizeRemoteTicketText(*req.Description, "description", maxRemoteTicketDescriptionLength, false)
		if err != nil {
			return err
		}
		doc["content"] = value
		changed = true
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if !validRemoteTicketStatus(status) {
			return fmt.Errorf("%w: invalid status", ErrInvalidRemoteTicketInput)
		}
		doc["forge:status"] = status
		changed = true
	}
	if req.Priority != nil {
		priority := strings.TrimSpace(*req.Priority)
		if !validRemoteTicketPriority(priority) {
			return fmt.Errorf("%w: invalid priority", ErrInvalidRemoteTicketInput)
		}
		doc["forge:priority"] = priority
		changed = true
	}
	if req.Type != nil {
		ticketType := strings.TrimSpace(*req.Type)
		if !validRemoteTicketType(ticketType) {
			return fmt.Errorf("%w: invalid type", ErrInvalidRemoteTicketInput)
		}
		doc["forge:ticketType"] = ticketType
		changed = true
	}
	if req.IsResolved != nil {
		doc["forge:isResolved"] = *req.IsResolved
		changed = true
	}
	if !changed {
		return fmt.Errorf("%w: no fields", ErrInvalidRemoteTicketInput)
	}
	doc["updated"] = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// normalizeRemoteTicketText trims and bounds one remote ticket text field.
func normalizeRemoteTicketText(value string, field string, maxLength int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidRemoteTicketInput, field)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return "", fmt.Errorf("%w: %s is too long", ErrInvalidRemoteTicketInput, field)
	}
	return value, nil
}

// normalizeRemoteTicketStatus returns a supported status with a resolved-state fallback.
func normalizeRemoteTicketStatus(status string, resolved bool) string {
	status = strings.TrimSpace(status)
	if validRemoteTicketStatus(status) {
		return status
	}
	if resolved {
		return "done"
	}
	return "open"
}

// normalizeRemoteTicketPriority returns a supported priority with a medium fallback.
func normalizeRemoteTicketPriority(priority string) string {
	priority = strings.TrimSpace(priority)
	if validRemoteTicketPriority(priority) {
		return priority
	}
	return "medium"
}

// normalizeRemoteTicketType returns a supported ticket type with a task fallback.
func normalizeRemoteTicketType(ticketType string) string {
	ticketType = strings.TrimSpace(ticketType)
	if validRemoteTicketType(ticketType) {
		return ticketType
	}
	return "task"
}

// validRemoteTicketStatus reports whether a remote ticket status is supported locally.
func validRemoteTicketStatus(status string) bool {
	switch status {
	case "open", "in_progress", "review", "done":
		return true
	default:
		return false
	}
}

// validRemoteTicketPriority reports whether a remote ticket priority is supported locally.
func validRemoteTicketPriority(priority string) bool {
	switch priority {
	case "low", "medium", "high", "urgent":
		return true
	default:
		return false
	}
}

// validRemoteTicketType reports whether a remote ticket type is supported locally.
func validRemoteTicketType(ticketType string) bool {
	switch ticketType {
	case "epic", "task", "subtask":
		return true
	default:
		return false
	}
}

// remoteTicketRank returns the local hierarchy rank string for a remote ticket type.
func remoteTicketRank(ticketType string) string {
	switch ticketType {
	case "epic":
		return "3"
	case "subtask":
		return "1"
	default:
		return "2"
	}
}

// mustMarshalDocument serializes trusted in-memory ActivityPub maps for immediate API projections.
func mustMarshalDocument(doc map[string]any) []byte {
	raw, _ := json.Marshal(doc)
	return raw
}

// resolveRemoteActor resolves the accepted public target formats for federation actions.
func (s *Service) resolveRemoteActor(ctx context.Context, resource string) (*remoteactor.Actor, error) {
	if s.resolver == nil {
		return nil, ErrRemoteActorUnavailable
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return nil, ErrInvalidRemoteResource
	}
	if strings.HasPrefix(strings.ToLower(resource), "acct:") {
		return s.discover(ctx, resource)
	}
	if isHTTPURL(resource) {
		return s.fetch(ctx, resource)
	}
	if strings.Contains(resource, "@") {
		return s.discover(ctx, "acct:"+resource)
	}
	return nil, ErrInvalidRemoteResource
}

// discover wraps remote actor discovery errors in API-facing sentinels.
func (s *Service) discover(ctx context.Context, resource string) (*remoteactor.Actor, error) {
	actor, err := s.resolver.Discover(ctx, resource)
	if err != nil {
		return nil, mapRemoteActorError(err)
	}
	return actor, nil
}

// fetch wraps remote actor URL fetch errors in API-facing sentinels.
func (s *Service) fetch(ctx context.Context, actorURL string) (*remoteactor.Actor, error) {
	actor, err := s.resolver.Fetch(ctx, actorURL)
	if err != nil {
		return nil, mapRemoteActorError(err)
	}
	return actor, nil
}

// mapRemoteActorError preserves specific remote actor failures for HTTP mapping.
func mapRemoteActorError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, remoteactor.ErrInvalidResource),
		errors.Is(err, remoteactor.ErrInvalidWebFinger),
		errors.Is(err, remoteactor.ErrInvalidActorDocument):
		return fmt.Errorf("%w: %v", ErrInvalidRemoteResource, err)
	case errors.Is(err, remoteactor.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrRemoteActorUnavailable, err)
	case errors.Is(err, remoteactor.ErrLocalActorConflict):
		return fmt.Errorf("%w: %v", ErrRemoteActorUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", ErrRemoteActorUnavailable, err)
	}
}

// isHTTPURL reports whether raw is an absolute HTTP(S) URL.
func isHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

// remoteActorProjection converts a cached remote actor to the public API shape.
func remoteActorProjection(actor *remoteactor.Actor) *RemoteActor {
	if actor == nil {
		return nil
	}
	return &RemoteActor{
		ID:                actor.ID,
		APID:              actor.APID,
		Type:              actor.Type,
		PreferredUsername: actor.PreferredUsername,
		Handle:            actor.Handle,
		Name:              actor.Name,
		Summary:           actor.Summary,
		InboxURL:          actor.InboxURL,
		OutboxURL:         actor.OutboxURL,
		FollowersURL:      actor.FollowersURL,
		FollowingURL:      actor.FollowingURL,
		LastFetchedAt:     actor.LastFetchedAt,
		CreatedAt:         actor.CreatedAt,
		UpdatedAt:         actor.UpdatedAt,
	}
}

// followDeliveryProjection converts delivery internals to the follow API shape.
func followDeliveryProjection(delivery *delivery.Delivery) *FollowDelivery {
	if delivery == nil {
		return nil
	}
	return &FollowDelivery{
		ID:             delivery.ID,
		ActivityAPID:   delivery.ActivityAPID,
		TargetInboxURL: delivery.TargetInboxURL,
		State:          delivery.State,
		CreatedAt:      delivery.CreatedAt,
		UpdatedAt:      delivery.UpdatedAt,
	}
}

// normalizeListOptions bounds pagination and validates optional state filters.
func normalizeListOptions(options ListOptions, allowState bool) (ListOptions, error) {
	if options.Limit <= 0 {
		options.Limit = defaultListLimit
	}
	if options.Limit > maxListLimit {
		options.Limit = maxListLimit
	}
	if options.Offset < 0 {
		return ListOptions{}, fmt.Errorf("%w: offset must be non-negative", ErrInvalidFilter)
	}
	options.State = strings.TrimSpace(options.State)
	if options.State != "" {
		if !allowState {
			return ListOptions{}, fmt.Errorf("%w: state filter is not supported", ErrInvalidFilter)
		}
		switch options.State {
		case "pending", "accepted", "rejected":
		default:
			return ListOptions{}, fmt.Errorf("%w: invalid follow state", ErrInvalidFilter)
		}
	}
	return options, nil
}

package federation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
)

const (
	// defaultListLimit is the fallback personal federation page size.
	defaultListLimit = 100
	// maxListLimit is the largest personal federation page size.
	maxListLimit = 500
)

// Repository defines persistence operations for personal federation views.
type Repository interface {
	ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error)
	ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error)
	ListRemoteProjectInvites(ctx context.Context, userID string, options ListOptions) ([]RemoteProjectInvite, error)
	GetRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInvite, error)
	LocalUserActor(ctx context.Context, userID string) (*LocalActor, error)
	StoreOutgoingFollow(ctx context.Context, actorID, remoteActorID, activityID, activityAPID, remoteActorAPID string, document []byte) (*RemoteFollow, bool, error)
	AcceptRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error)
	RejectRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error)
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

// Service exposes authenticated personal federation views.
type Service struct {
	repo     Repository
	resolver RemoteActorResolver
	delivery DeliveryEnqueuer
	cfg      activitypub.Config
}

// Option configures the personal federation service.
type Option func(*Service)

// NewService creates a personal federation service.
func NewService(repo Repository, opts ...Option) *Service {
	service := &Service{
		repo: repo,
		cfg:  activitypub.NewConfig("", ""),
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

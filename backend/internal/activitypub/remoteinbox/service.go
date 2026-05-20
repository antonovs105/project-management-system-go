package remoteinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
)

const DefaultMaxBodyBytes int64 = 1 << 20

type Verifier interface {
	VerifyRequest(ctx context.Context, req *http.Request, body []byte) (*httpsig.VerifiedRequest, error)
}

type Service struct {
	repo         Repository
	verifier     Verifier
	delivery     DeliveryEnqueuer
	maxBodyBytes int64
}

type DeliveryEnqueuer interface {
	Enqueue(ctx context.Context, activityID string, targetInboxURL string) (*delivery.Delivery, error)
}

type Option func(*Service)

func NewService(repo Repository, verifier Verifier, opts ...Option) *Service {
	service := &Service{
		repo:         repo,
		verifier:     verifier,
		maxBodyBytes: DefaultMaxBodyBytes,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func WithMaxBodyBytes(size int64) Option {
	return func(s *Service) {
		if size > 0 {
			s.maxBodyBytes = size
		}
	}
}

func WithDelivery(delivery DeliveryEnqueuer) Option {
	return func(s *Service) {
		s.delivery = delivery
	}
}

func (s *Service) MaxBodyBytes() int64 {
	return s.maxBodyBytes
}

func (s *Service) Receive(ctx context.Context, req *http.Request, targetAPID string, body []byte) (*AcceptedActivity, error) {
	targetActorID, err := s.repo.FindLocalActorIDByAPID(ctx, targetAPID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, err
	}

	verified, err := s.verifier.VerifyRequest(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	activity, err := parseActivity(body)
	if err != nil {
		return nil, err
	}
	activity.ActorID = verified.ActorID

	signatureActorAPID := verified.ActorAPID
	if signatureActorAPID == "" {
		signatureActorAPID, err = s.repo.FindActorAPIDByID(ctx, verified.ActorID)
		if err != nil {
			return nil, err
		}
	}
	if activity.ActorAPID != signatureActorAPID {
		return nil, ErrForbiddenActor
	}

	isProjectFollow := false
	if activity.Type == "Follow" {
		if activity.ObjectAPID == nil || *activity.ObjectAPID != targetAPID {
			return nil, fmt.Errorf("%w: follow object must match inbox actor", ErrInvalidActivity)
		}
		isProjectActor, err := s.repo.IsProjectActor(ctx, targetActorID)
		if err != nil {
			return nil, err
		}
		isProjectFollow = isProjectActor
	}

	accepted, err := s.repo.StoreInboundActivity(ctx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if isProjectFollow && !accepted.Duplicate {
		response, err := s.repo.AcceptProjectFollow(ctx, targetActorID, activity)
		if err != nil {
			return nil, err
		}
		accepted.ResponseActivityID = response.ActivityID
		accepted.ResponseActivityAPID = response.ActivityAPID
		s.enqueueFollowResponse(ctx, response)
	}
	return accepted, nil
}

func (s *Service) enqueueFollowResponse(ctx context.Context, response *FollowResponse) {
	if s.delivery == nil || response == nil || response.ActivityID == "" || response.TargetInboxURL == "" {
		return
	}
	if _, err := s.delivery.Enqueue(ctx, response.ActivityID, response.TargetInboxURL); err != nil {
		log.Printf("failed to enqueue ActivityPub Accept{Follow} delivery to %s: %v", response.TargetInboxURL, err)
	}
}

func parseActivity(body []byte) (*InboundActivity, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActivity, err)
	}

	activityID := extractAPID(raw["id"])
	if activityID == "" || !isAbsoluteURI(activityID) {
		return nil, fmt.Errorf("%w: id", ErrInvalidActivity)
	}

	activityType, err := extractActivityType(raw["type"])
	if err != nil {
		return nil, err
	}

	actorAPID := extractAPID(raw["actor"])
	if actorAPID == "" || !isAbsoluteURI(actorAPID) {
		return nil, fmt.Errorf("%w: actor", ErrInvalidActivity)
	}

	activity := &InboundActivity{
		ID:        activityID,
		Type:      activityType,
		ActorAPID: actorAPID,
		Document:  append([]byte(nil), body...),
	}
	if objectAPID := extractAPID(raw["object"]); objectAPID != "" {
		activity.ObjectAPID = &objectAPID
	}
	if targetAPID := extractAPID(raw["target"]); targetAPID != "" {
		activity.TargetAPID = &targetAPID
	}
	return activity, nil
}

func extractActivityType(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if isSupportedActivityType(typed) {
			return typed, nil
		}
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok && isSupportedActivityType(value) {
				return value, nil
			}
		}
	}
	return "", ErrUnsupportedActivity
}

func isSupportedActivityType(value string) bool {
	switch value {
	case "Create", "Update", "Delete", "Add", "Remove", "Invite", "Accept", "Reject", "Follow", "Undo":
		return true
	default:
		return false
	}
}

func extractAPID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if id, ok := typed["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func isAbsoluteURI(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func isActivityMediaType(value string) bool {
	if value == "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	return mediaType == "application/activity+json" || mediaType == "application/ld+json"
}

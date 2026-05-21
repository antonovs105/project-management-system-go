package webfinger

import (
	"context"
	"database/sql"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
)

const (
	// relSelf is the WebFinger link relation for the actor document.
	relSelf = "self"
	// activityJSONMediaTyp is the ActivityPub actor document media type.
	activityJSONMediaTyp = "application/activity+json"
)

// Service resolves local actor handles into WebFinger JRD documents.
type Service struct {
	repo Repository
	cfg  activitypub.Config
}

// NewService creates a WebFinger service.
func NewService(repo Repository, cfg activitypub.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// Resolve returns a JRD document for an acct: resource on the local domain.
func (s *Service) Resolve(ctx context.Context, resource string) (*JRD, error) {
	username, domain, err := parseAcctResource(resource)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(domain, s.cfg.LocalDomain) {
		return nil, ErrNotFound
	}

	actor, err := s.repo.FindLocalActor(ctx, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &JRD{
		Subject: "acct:" + actor.Username + "@" + s.cfg.LocalDomain,
		Aliases: []string{actor.APID},
		Links: []Link{
			{
				Rel:  relSelf,
				Type: activityJSONMediaTyp,
				Href: actor.APID,
			},
		},
	}, nil
}

// parseAcctResource validates and splits an acct: WebFinger resource.
func parseAcctResource(resource string) (username string, domain string, err error) {
	resource = strings.TrimSpace(resource)
	if resource == "" || !strings.HasPrefix(strings.ToLower(resource), "acct:") {
		return "", "", ErrInvalidResource
	}

	account := resource[len("acct:"):]
	username, domain, ok := strings.Cut(account, "@")
	if !ok || username == "" || domain == "" {
		return "", "", ErrInvalidResource
	}
	if strings.Contains(username, "/") || strings.ContainsAny(username, " \t\r\n") {
		return "", "", ErrInvalidResource
	}
	if strings.ContainsAny(domain, " \t\r\n") {
		return "", "", ErrInvalidResource
	}

	return username, domain, nil
}

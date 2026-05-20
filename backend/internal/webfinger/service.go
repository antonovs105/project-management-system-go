package webfinger

import (
	"context"
	"database/sql"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
)

const (
	relSelf              = "self"
	activityJSONMediaTyp = "application/activity+json"
)

type Service struct {
	repo Repository
	cfg  activitypub.Config
}

func NewService(repo Repository, cfg activitypub.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) Resolve(ctx context.Context, resource string) (*JRD, error) {
	username, domain, err := parseAcctResource(resource)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(domain, s.cfg.LocalDomain) {
		return nil, ErrNotFound
	}

	actor, err := s.repo.FindLocalUserActor(ctx, username)
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

package activitypub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
)

var (
	ErrMissingAuthorization = errors.New("missing activitypub authorization")
	ErrInvalidAuthorization = errors.New("invalid activitypub authorization")
	ErrAccessDenied         = errors.New("activitypub access denied")
)

type RequestVerifier interface {
	VerifyActorID(ctx context.Context, req *http.Request) (string, error)
}

type AccessAuthorizer struct {
	db        *sqlx.DB
	jwtSecret []byte
	verifier  RequestVerifier
}

func NewAccessAuthorizer(db *sqlx.DB, jwtSecret []byte, verifier RequestVerifier) *AccessAuthorizer {
	return &AccessAuthorizer{db: db, jwtSecret: jwtSecret, verifier: verifier}
}

func (a *AccessAuthorizer) AuthorizeActor(ctx context.Context, req *http.Request, actorID string) error {
	credentialActorID, err := a.credentialActorID(ctx, req)
	if err != nil {
		return err
	}
	if credentialActorID != actorID {
		return ErrAccessDenied
	}
	return nil
}

func (a *AccessAuthorizer) AuthorizeProject(ctx context.Context, req *http.Request, projectID string) error {
	actorID, err := a.credentialActorID(ctx, req)
	if err != nil {
		return err
	}

	var allowed bool
	if err := a.db.GetContext(ctx, &allowed, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE user_id = $1 AND project_id = $2
		) OR EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, actorID, projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccessDenied
		}
		return err
	}
	if !allowed {
		return ErrAccessDenied
	}
	return nil
}

func (a *AccessAuthorizer) credentialActorID(ctx context.Context, req *http.Request) (string, error) {
	if req == nil {
		return "", ErrMissingAuthorization
	}
	if actorID, ok, err := a.actorIDFromJWT(req); ok || err != nil {
		return actorID, err
	}
	if a.verifier != nil && strings.TrimSpace(req.Header.Get("Signature-Input")) != "" {
		actorID, err := a.verifier.VerifyActorID(ctx, req)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidAuthorization, err)
		}
		if actorID == "" {
			return "", ErrInvalidAuthorization
		}
		return actorID, nil
	}
	return "", ErrMissingAuthorization
}

func (a *AccessAuthorizer) actorIDFromJWT(req *http.Request) (actorID string, ok bool, err error) {
	if len(a.jwtSecret) == 0 {
		return "", false, nil
	}
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader == "" {
		return "", false, nil
	}
	tokenValue, found := strings.CutPrefix(authHeader, "Bearer ")
	if !found || strings.TrimSpace(tokenValue) == "" {
		return "", true, ErrInvalidAuthorization
	}

	token, err := jwt.Parse(strings.TrimSpace(tokenValue), func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidAuthorization
		}
		return a.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", true, ErrInvalidAuthorization
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", true, ErrInvalidAuthorization
	}
	sub, ok := claims["sub"].(string)
	if !ok || strings.TrimSpace(sub) == "" {
		return "", true, ErrInvalidAuthorization
	}
	return sub, true, nil
}

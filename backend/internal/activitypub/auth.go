package activitypub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
)

var (
	// ErrMissingAuthorization reports that a protected ActivityPub read has no usable credential.
	ErrMissingAuthorization = errors.New("missing activitypub authorization")
	// ErrInvalidAuthorization reports that an ActivityPub read credential is malformed or rejected.
	ErrInvalidAuthorization = errors.New("invalid activitypub authorization")
	// ErrAccessDenied reports that a valid credential is not allowed to read the target resource.
	ErrAccessDenied = errors.New("activitypub access denied")
)

// JWTTokenValidator validates whether a bearer JWT token_version claim is current.
type JWTTokenValidator interface {
	ValidateTokenVersion(ctx context.Context, userID string, tokenVersion int) error
}

// RequestVerifier verifies HTTP signatures and returns the authenticated actor ID.
type RequestVerifier interface {
	VerifyActorID(ctx context.Context, req *http.Request) (string, error)
}

// AccessAuthorizer authorizes local JWT and remote HTTP-signature reads.
type AccessAuthorizer struct {
	db           *sqlx.DB
	jwtSecret    []byte
	verifier     RequestVerifier
	jwtValidator JWTTokenValidator
}

// NewAccessAuthorizer creates an ActivityPub read authorizer.
func NewAccessAuthorizer(db *sqlx.DB, jwtSecret []byte, verifier RequestVerifier, validators ...JWTTokenValidator) *AccessAuthorizer {
	var validator JWTTokenValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return &AccessAuthorizer{db: db, jwtSecret: jwtSecret, verifier: verifier, jwtValidator: validator}
}

// AuthorizeActor requires the credential actor to match the requested actor.
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

// AuthorizeProject requires the credential actor to belong to or follow a project.
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

// credentialActorID resolves the actor authenticated by a local JWT or remote signature.
func (a *AccessAuthorizer) credentialActorID(ctx context.Context, req *http.Request) (string, error) {
	if req == nil {
		return "", ErrMissingAuthorization
	}
	if actorID, ok, err := a.actorIDFromJWT(ctx, req); ok || err != nil {
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

// actorIDFromJWT extracts and optionally validates a local JWT actor credential.
func (a *AccessAuthorizer) actorIDFromJWT(ctx context.Context, req *http.Request) (actorID string, ok bool, err error) {
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
		if token.Method == nil || token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
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
	if a.jwtValidator != nil {
		tokenVersion, ok := jwtTokenVersionFromClaims(claims)
		if !ok {
			return "", true, ErrInvalidAuthorization
		}
		if err := a.jwtValidator.ValidateTokenVersion(ctx, sub, tokenVersion); err != nil {
			return "", true, ErrInvalidAuthorization
		}
	}
	return sub, true, nil
}

// jwtTokenVersionFromClaims reads the token_version claim without accepting fractional values.
func jwtTokenVersionFromClaims(claims jwt.MapClaims) (int, bool) {
	raw, ok := claims["token_version"]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		maxInt := int(^uint(0) >> 1)
		if value <= 0 || math.Trunc(value) != value || value > float64(maxInt) {
			return 0, false
		}
		return int(value), true
	case int:
		return value, value > 0
	case int64:
		maxInt := int64(int(^uint(0) >> 1))
		if value <= 0 || value > maxInt {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}

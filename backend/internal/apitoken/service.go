package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidInput reports malformed token lifecycle input.
var ErrInvalidInput = errors.New("invalid api token input")

// RepositoryStore is the persistence contract for API tokens.
type RepositoryStore interface {
	Create(context.Context, *Token, []byte) error
	List(context.Context, string) ([]Token, error)
	Revoke(context.Context, string, string) error
	Authenticate(context.Context, []byte, time.Time) (string, []string, error)
}

// RoleProvider resolves instance roles for privileged scopes.
type RoleProvider interface {
	InstanceRole(context.Context, string) (string, error)
}

// Service validates and manages API token credentials.
type Service struct {
	repository RepositoryStore
	roles      RoleProvider
	now        func() time.Time
}

// NewService creates an API token service.
func NewService(repository RepositoryStore, roles RoleProvider) *Service {
	return &Service{repository: repository, roles: roles, now: time.Now}
}

// Create generates a secret once and persists only its SHA-256 digest.
func (s *Service) Create(ctx context.Context, userID string, request CreateRequest) (*CreatedToken, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, invalidInput("valid user id is required")
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 80 {
		return nil, invalidInput("name must contain 1 to 80 characters")
	}
	scopes, err := normalizeScopes(request.Scopes)
	if err != nil {
		return nil, err
	}
	if contains(scopes, ScopeAdmin) {
		role, err := s.roles.InstanceRole(ctx, userID)
		if err != nil {
			return nil, err
		}
		if role != "owner" && role != "admin" {
			return nil, invalidInput("admin scope requires an instance admin or owner")
		}
	}
	now := s.now().UTC()
	if request.ExpiresAt != nil {
		expires := request.ExpiresAt.UTC()
		if !expires.After(now) || expires.After(now.AddDate(1, 0, 0)) {
			return nil, invalidInput("expiry must be in the future and no more than one year away")
		}
		request.ExpiresAt = &expires
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := "progo_" + base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(secret))
	value := Token{UserID: userID, Name: request.Name, Prefix: secret[:14], Scopes: scopes, ExpiresAt: request.ExpiresAt}
	if err := s.repository.Create(ctx, &value, digest[:]); err != nil {
		return nil, err
	}
	return &CreatedToken{Token: value, Secret: secret}, nil
}

// List returns credential metadata without token secrets.
func (s *Service) List(ctx context.Context, userID string) ([]Token, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, invalidInput("valid user id is required")
	}
	return s.repository.List(ctx, userID)
}

// Revoke invalidates an owned credential.
func (s *Service) Revoke(ctx context.Context, userID, tokenID string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return invalidInput("valid user id is required")
	}
	if _, err := uuid.Parse(tokenID); err != nil {
		return invalidInput("valid token id is required")
	}
	return s.repository.Revoke(ctx, userID, tokenID)
}

// AuthenticateCredential implements middleware credential authentication.
func (s *Service) AuthenticateCredential(ctx context.Context, raw string) (string, []string, error) {
	if !strings.HasPrefix(raw, "progo_") || len(raw) < 20 {
		return "", nil, ErrNotFound
	}
	digest := sha256.Sum256([]byte(raw))
	return s.repository.Authenticate(ctx, digest[:], s.now().UTC())
}

func normalizeScopes(values []string) ([]string, error) {
	allowed := map[string]bool{ScopeProjectsRead: true, ScopeProjectsWrite: true, ScopeAccountRead: true, ScopeAccountWrite: true, ScopeTokensManage: true, ScopeAdmin: true}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !allowed[value] {
			return nil, invalidInput(fmt.Sprintf("unsupported scope %q", value))
		}
		unique[value] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, invalidInput("at least one scope is required")
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func invalidInput(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }

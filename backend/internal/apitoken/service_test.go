package apitoken

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryRepository struct {
	created Token
	hash    []byte
}

func (r *memoryRepository) Create(_ context.Context, value *Token, hash []byte) error {
	r.created = *value
	r.hash = append([]byte(nil), hash...)
	value.ID = "22222222-2222-4222-8222-222222222222"
	value.CreatedAt = time.Unix(100, 0).UTC()
	return nil
}
func (r *memoryRepository) List(context.Context, string) ([]Token, error) { return nil, nil }
func (r *memoryRepository) Revoke(context.Context, string, string) error  { return nil }
func (r *memoryRepository) Authenticate(context.Context, []byte, time.Time) (string, []string, error) {
	return "11111111-1111-4111-8111-111111111111", []string{ScopeProjectsRead}, nil
}

type roleProviderStub struct{ role string }

func (s roleProviderStub) InstanceRole(context.Context, string) (string, error) { return s.role, nil }

func TestServiceCreateStoresOnlyHashAndNormalizesScopes(t *testing.T) {
	repository := &memoryRepository{}
	service := NewService(repository, roleProviderStub{role: "user"})
	service.now = func() time.Time { return time.Unix(10, 0).UTC() }

	created, err := service.Create(context.Background(), "11111111-1111-4111-8111-111111111111", CreateRequest{
		Name:   " Build automation ",
		Scopes: []string{ScopeProjectsWrite, ScopeProjectsRead, ScopeProjectsRead},
	})

	require.NoError(t, err)
	require.Contains(t, created.Secret, "progo_")
	require.Equal(t, "Build automation", repository.created.Name)
	require.Equal(t, []string{ScopeProjectsRead, ScopeProjectsWrite}, repository.created.Scopes)
	require.Equal(t, created.Secret[:14], repository.created.Prefix)
	digest := sha256.Sum256([]byte(created.Secret))
	require.Equal(t, digest[:], repository.hash)
	require.NotContains(t, string(repository.hash), created.Secret)
}

func TestServiceCreateRestrictsAdminScope(t *testing.T) {
	service := NewService(&memoryRepository{}, roleProviderStub{role: "user"})

	created, err := service.Create(context.Background(), "11111111-1111-4111-8111-111111111111", CreateRequest{Name: "admin", Scopes: []string{ScopeAdmin}})

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, created)
}

func TestServiceCreateBoundsExpiry(t *testing.T) {
	service := NewService(&memoryRepository{}, roleProviderStub{role: "owner"})
	now := time.Unix(10, 0).UTC()
	service.now = func() time.Time { return now }
	expires := now.AddDate(1, 0, 0).Add(time.Second)

	created, err := service.Create(context.Background(), "11111111-1111-4111-8111-111111111111", CreateRequest{Name: "long lived", Scopes: []string{ScopeProjectsRead}, ExpiresAt: &expires})

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, created)
}

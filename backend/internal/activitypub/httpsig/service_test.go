package httpsig

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryRepository struct {
	key *ActorKey
}

func (m memoryRepository) ActivePrivateKey(ctx context.Context, actorID string) (*ActorKey, error) {
	return m.key, nil
}

func (m memoryRepository) PublicKeyByKeyID(ctx context.Context, keyID string) (*ActorKey, error) {
	return m.key, nil
}

type resolvingRepository struct {
	key *ActorKey
}

func (r *resolvingRepository) ActivePrivateKey(ctx context.Context, actorID string) (*ActorKey, error) {
	if r.key == nil {
		return nil, sql.ErrNoRows
	}
	return r.key, nil
}

func (r *resolvingRepository) PublicKeyByKeyID(ctx context.Context, keyID string) (*ActorKey, error) {
	if r.key == nil {
		return nil, sql.ErrNoRows
	}
	return r.key, nil
}

func TestSignAndVerifyRequest(t *testing.T) {
	service, key, now := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://Remote.TEST/inbox?attempt=1", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	assert.Equal(t, now.Format(http.TimeFormat), req.Header.Get("Date"))
	assert.Equal(t, contentDigest(body), req.Header.Get("Content-Digest"))
	assert.Contains(t, req.Header.Get("Signature-Input"), `keyid="https://example.test/users/alice#main-key"`)
	assert.Contains(t, req.Header.Get("Signature-Input"), `alg="rsa-v1_5-sha256"`)
	assert.Contains(t, req.Header.Get("Signature"), "ap=:")

	verified, err := service.VerifyRequest(context.Background(), req, body)
	require.NoError(t, err)
	assert.Equal(t, key.ActorID, verified.ActorID)
	assert.Equal(t, key.KeyID, verified.KeyID)
	assert.Equal(t, AlgorithmRSAV15SHA256, verified.Algorithm)
	assert.Equal(t, now.Unix(), verified.CreatedAt.Unix())
	assert.Equal(t, []string{"@method", "@authority", "@path", "@query", "date", "content-digest"}, verified.Components)
}

func TestSignRequestNormalizesLegacyAlgorithm(t *testing.T) {
	service, key, _ := newTestService(t, legacyRSAAlgorithm)
	req := newSignedRequest(t, http.MethodGet, "https://remote.test/users/bob", nil)

	err := service.SignRequest(context.Background(), key.ActorID, req, nil)
	require.NoError(t, err)

	assert.Contains(t, req.Header.Get("Signature-Input"), `alg="rsa-v1_5-sha256"`)
	assert.Empty(t, req.Header.Get("Content-Digest"))
}

func TestVerifyRequestRejectsTamperedBody(t *testing.T) {
	service, key, _ := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	_, err = service.VerifyRequest(context.Background(), req, []byte(`{"type":"Delete"}`))
	require.ErrorIs(t, err, ErrInvalidDigest)
}

func TestVerifyRequestRequiresBodyDigestCoverage(t *testing.T) {
	service, key, _ := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, nil)
	require.NoError(t, err)

	_, err = service.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrMissingCoveredContent)
}

func TestVerifyRequestRejectsInvalidSignedDate(t *testing.T) {
	service, key, _ := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)
	req.Header.Set("Date", "not-a-date")

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	_, err = service.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrInvalidDate)
}

func TestVerifyRequestRejectsExpiredSignedDate(t *testing.T) {
	service, key, now := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)
	req.Header.Set("Date", now.Add(-10*time.Minute).Format(http.TimeFormat))

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	_, err = service.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrExpiredSignature)
}

func TestVerifyRequestRejectsTamperedSignedComponent(t *testing.T) {
	service, key, _ := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	req.URL.Path = "/other-inbox"
	_, err = service.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrInvalidSignature)
}

func TestVerifyRequestRejectsTamperedQuery(t *testing.T) {
	service, key, _ := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox?attempt=1", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	req.URL.RawQuery = "attempt=2"
	_, err = service.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrInvalidSignature)
}

func TestVerifyRequestRejectsExpiredSignature(t *testing.T) {
	service, key, now := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Create"}`)
	req := newSignedRequest(t, http.MethodPost, "https://remote.test/inbox", body)

	err := service.SignRequest(context.Background(), key.ActorID, req, body)
	require.NoError(t, err)

	expiredVerifier := NewService(
		memoryRepository{key: key},
		WithClock(func() time.Time { return now.Add(10 * time.Minute) }),
		WithMaxAge(5*time.Minute),
	)
	_, err = expiredVerifier.VerifyRequest(context.Background(), req, body)
	require.ErrorIs(t, err, ErrExpiredSignature)
}

func TestVerifyRequestRequiresSignatureHeaders(t *testing.T) {
	service, _, _ := newTestService(t, AlgorithmRSAV15SHA256)
	req := newSignedRequest(t, http.MethodGet, "https://remote.test/users/bob", nil)

	_, err := service.VerifyRequest(context.Background(), req, nil)
	require.ErrorIs(t, err, ErrMissingSignature)
}

func TestVerifyRequestResolvesMissingPublicKey(t *testing.T) {
	signer, key, now := newTestService(t, AlgorithmRSAV15SHA256)
	body := []byte(`{"type":"Follow"}`)
	req := newSignedRequest(t, http.MethodPost, "https://local.test/projects/1/inbox", body)
	require.NoError(t, signer.SignRequest(context.Background(), key.ActorID, req, body))

	repo := &resolvingRepository{}
	resolved := false
	verifier := NewService(
		repo,
		WithClock(func() time.Time { return now }),
		WithMaxAge(5*time.Minute),
		WithMissingKeyResolver(func(ctx context.Context, keyID string) error {
			resolved = true
			repo.key = key
			return nil
		}),
	)

	verified, err := verifier.VerifyRequest(context.Background(), req, body)

	require.NoError(t, err)
	assert.True(t, resolved)
	assert.Equal(t, key.KeyID, verified.KeyID)
}

func newTestService(t *testing.T, algorithm string) (*Service, *ActorKey, time.Time) {
	t.Helper()

	publicPEM, privatePEM, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	key := &ActorKey{
		ActorID:       "actor-1",
		KeyID:         "https://example.test/users/alice#main-key",
		Algorithm:     algorithm,
		PublicKeyPEM:  publicPEM,
		PrivateKeyPEM: privatePEM,
	}
	now := time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC)
	service := NewService(
		memoryRepository{key: key},
		WithClock(func() time.Time { return now }),
		WithMaxAge(5*time.Minute),
	)
	return service, key, now
}

func newSignedRequest(t *testing.T, method, rawURL string, body []byte) *http.Request {
	t.Helper()

	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, rawURL, reader)
	require.NoError(t, err)
	return req
}

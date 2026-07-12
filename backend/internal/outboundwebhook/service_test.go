package outboundwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/stretchr/testify/require"
)

type authorizerStub struct{ allowed bool }

func (s authorizerStub) HasProjectPermission(context.Context, string, string, string) (bool, error) {
	return s.allowed, nil
}

type serviceRepositoryStub struct {
	created      Webhook
	secretCipher string
}

func (r *serviceRepositoryStub) Create(_ context.Context, value *Webhook, secret string) error {
	r.created = *value
	r.secretCipher = secret
	value.ID = "33333333-3333-4333-8333-333333333333"
	return nil
}
func (r *serviceRepositoryStub) List(context.Context, string) ([]Webhook, error) { return nil, nil }
func (r *serviceRepositoryStub) Delete(context.Context, string, string) error    { return nil }
func (r *serviceRepositoryStub) ListDeliveries(context.Context, string, int) ([]Delivery, error) {
	return nil, nil
}
func (r *serviceRepositoryStub) Retry(context.Context, string, string) error { return nil }

func TestServiceRejectsPrivateCallbackByDefault(t *testing.T) {
	service := NewService(&serviceRepositoryStub{}, authorizerStub{allowed: true}, secrets.NoopPrivateKeyCodec{})

	created, err := service.Create(context.Background(), "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "unsafe", "http://127.0.0.1/hook", []string{"ticket.created"})

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Nil(t, created)
}

func TestServiceCreatesEncryptedOneTimeSigningSecret(t *testing.T) {
	repository := &serviceRepositoryStub{}
	codec, err := secrets.NewPrivateKeyCodec("01234567890123456789012345678901")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	service := NewService(repository, authorizerStub{allowed: true}, codec, WithAllowPrivateNetworks(true))

	created, err := service.Create(context.Background(), "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", " automation ", server.URL, []string{"ticket.created", "ticket.created"})

	require.NoError(t, err)
	require.Equal(t, "automation", created.Name)
	require.Equal(t, []string{"ticket.created"}, created.Events)
	require.Contains(t, created.Secret, "whsec_")
	require.NotEqual(t, created.Secret, repository.secretCipher)
	plaintext, err := codec.DecryptPrivateKey(repository.secretCipher)
	require.NoError(t, err)
	require.Equal(t, created.Secret, plaintext)
}

type dispatchRepositoryStub struct {
	delivery  *Delivery
	claimed   bool
	completed bool
	failed    bool
	status    *int
}

func (r *dispatchRepositoryStub) EnqueueActivityEvents(context.Context) (int, error) { return 1, nil }
func (r *dispatchRepositoryStub) Claim(context.Context, time.Time) (*Delivery, error) {
	if r.claimed {
		return nil, ErrNotFound
	}
	r.claimed = true
	value := *r.delivery
	value.Attempts++
	return &value, nil
}
func (r *dispatchRepositoryStub) Complete(context.Context, string, int, time.Time) error {
	r.completed = true
	return nil
}
func (r *dispatchRepositoryStub) Fail(_ context.Context, _ *Delivery, status *int, _ string, _ time.Time) error {
	r.failed = true
	r.status = status
	return nil
}

func TestDispatcherSignsAndCompletesDelivery(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte(`{"event":"ticket.created"}`)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "ticket.created", request.Header.Get("X-Progo-Event"))
		require.Equal(t, "delivery-1", request.Header.Get("X-Progo-Delivery"))
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(payload)
		require.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), request.Header.Get("X-Progo-Signature-256"))
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	repository := &dispatchRepositoryStub{delivery: &Delivery{ID: "delivery-1", TargetURL: server.URL, EventType: "ticket.created", Payload: payload, SecretCipher: secret, MaxAttempts: 8}}
	dispatcher := NewDispatcher(repository, secrets.NoopPrivateKeyCodec{}, WithHTTPClient(server.Client()))

	processed, err := dispatcher.DispatchOnce(context.Background(), 10)

	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.True(t, repository.completed)
	require.False(t, repository.failed)
}

package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workerRepo struct {
	delivery     *Delivery
	deliveredID  string
	failedID     string
	failedMsg    string
	nextAttempt  *time.Time
	startAttempt error
}

func (r *workerRepo) Create(ctx context.Context, activityID string, targetInboxURL string, maxAttempts int) (*Delivery, bool, error) {
	return nil, false, nil
}

func (r *workerRepo) StartAttempt(ctx context.Context, deliveryID string) (*Delivery, error) {
	if r.startAttempt != nil {
		return nil, r.startAttempt
	}
	return r.delivery, nil
}

func (r *workerRepo) MarkDelivered(ctx context.Context, deliveryID string) error {
	r.deliveredID = deliveryID
	return nil
}

func (r *workerRepo) MarkFailed(ctx context.Context, deliveryID string, message string, nextAttemptAt *time.Time) error {
	r.failedID = deliveryID
	r.failedMsg = message
	r.nextAttempt = nextAttemptAt
	return nil
}

type signerFunc func(ctx context.Context, actorID string, req *http.Request, body []byte) error

func (f signerFunc) SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error {
	return f(ctx, actorID, req, body)
}

type httpClientFunc func(req *http.Request) (*http.Response, error)

func (f httpClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type staticKeyRepo struct {
	key *httpsig.ActorKey
}

func (r staticKeyRepo) ActivePrivateKey(ctx context.Context, actorID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func (r staticKeyRepo) PublicKeyByKeyID(ctx context.Context, keyID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func TestWorkerDeliversActivity(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(1, 3)}
	var signedActorID string
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		signedActorID = actorID
		req.Header.Set("Signature", "ap=:signed:")
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://remote.example/inbox", req.URL.String())
		assert.Equal(t, "application/activity+json", req.Header.Get("Content-Type"))
		assert.Equal(t, "ap=:signed:", req.Header.Get("Signature"))
		return response(http.StatusAccepted, ""), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.NoError(t, err)
	assert.Equal(t, "local-actor", signedActorID)
	assert.Equal(t, "delivery-1", repo.deliveredID)
	assert.Empty(t, repo.failedID)
}

func TestWorkerDeliversVerifiableSignedActivity(t *testing.T) {
	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	key := &httpsig.ActorKey{
		ActorID:       "local-actor",
		ActorAPID:     "https://local.example/users/alice",
		KeyID:         "https://local.example/users/alice#main-key",
		Algorithm:     httpsig.AlgorithmRSAV15SHA256,
		PublicKeyPEM:  publicKey,
		PrivateKeyPEM: privateKey,
	}
	verifier := httpsig.NewService(staticKeyRepo{key: key})

	received := false
	client := httpClientFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "application/activity+json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/activity+json", r.Header.Get("Accept"))
		assert.Equal(t, deliveryUserAgent, r.Header.Get("User-Agent"))
		assert.NotEmpty(t, r.Header.Get("Date"))
		assert.NotEmpty(t, r.Header.Get("Content-Digest"))
		assert.NotEmpty(t, r.Header.Get("Signature-Input"))
		assert.NotEmpty(t, r.Header.Get("Signature"))

		receivedBody, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		verified, verifyErr := verifier.VerifyRequest(context.Background(), r, receivedBody)
		require.NoError(t, verifyErr)
		assert.Equal(t, key.ActorID, verified.ActorID)
		assert.Equal(t, key.ActorAPID, verified.ActorAPID)
		assert.Equal(t, []byte(testDelivery(1, 3).Document), receivedBody)
		received = true
		return response(http.StatusAccepted, ""), nil
	})

	delivery := testDelivery(1, 3)
	delivery.TargetInboxURL = "https://remote.example/inbox"
	repo := &workerRepo{delivery: delivery}
	worker := NewWorker(repo, httpsig.NewService(staticKeyRepo{key: key}), client)

	err = worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.NoError(t, err)
	assert.True(t, received)
	assert.Equal(t, "delivery-1", repo.deliveredID)
	assert.Empty(t, repo.failedID)
}

func TestWorkerRetriesTransientFailureWithBackoff(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(2, 5)}
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusInternalServerError, "try later"), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.False(t, errors.Is(err, asynq.SkipRetry))
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Contains(t, repo.failedMsg, "500")
	require.NotNil(t, repo.nextAttempt)
	assert.WithinDuration(t, time.Now().UTC().Add(2*time.Minute), *repo.nextAttempt, 5*time.Second)
}

func TestWorkerRetriesRateLimitFailure(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(1, 5)}
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusTooManyRequests, "slow down"), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.False(t, errors.Is(err, asynq.SkipRetry))
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Contains(t, repo.failedMsg, "429")
	require.NotNil(t, repo.nextAttempt)
}

func TestWorkerRetriesNetworkFailure(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(1, 5)}
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.False(t, errors.Is(err, asynq.SkipRetry))
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Equal(t, "connection refused", repo.failedMsg)
	require.NotNil(t, repo.nextAttempt)
}

func TestWorkerSkipsRetryForPermanentFailure(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(1, 5)}
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusNotFound, "gone"), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry))
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Nil(t, repo.nextAttempt)
}

func TestWorkerSkipsRetryForUnsupportedInboxScheme(t *testing.T) {
	delivery := testDelivery(1, 5)
	delivery.TargetInboxURL = "ftp://remote.example/inbox"
	repo := &workerRepo{delivery: delivery}
	clientCalled := false
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		clientCalled = true
		return response(http.StatusAccepted, ""), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry))
	assert.False(t, clientCalled)
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Contains(t, repo.failedMsg, "unsupported scheme")
	assert.Nil(t, repo.nextAttempt)
}

func TestWorkerSkipsRetryForUnsafeInboxHost(t *testing.T) {
	delivery := testDelivery(1, 5)
	delivery.TargetInboxURL = "http://127.0.0.1/inbox"
	repo := &workerRepo{delivery: delivery}
	signerCalled := false
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		signerCalled = true
		return nil
	}), nil)

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry))
	assert.False(t, signerCalled)
	assert.Equal(t, "delivery-1", repo.failedID)
	assert.Contains(t, repo.failedMsg, "blocked address")
	assert.Nil(t, repo.nextAttempt)
}

func TestWorkerStoresBoundedSingleLineResponseSnippet(t *testing.T) {
	repo := &workerRepo{delivery: testDelivery(1, 5)}
	worker := NewWorker(repo, signerFunc(func(ctx context.Context, actorID string, req *http.Request, body []byte) error {
		return nil
	}), httpClientFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusInternalServerError, strings.Repeat("try\nlater\t", 200)), nil
	}))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "delivery-1"))

	require.Error(t, err)
	assert.Contains(t, repo.failedMsg, "500")
	assert.NotContains(t, repo.failedMsg, "\n")
	assert.NotContains(t, repo.failedMsg, "\t")
	assert.LessOrEqual(t, len(repo.failedMsg), int(maxResponseSnippetBytes)+len("delivery failed: 500 Internal Server Error: ..."))
	assert.True(t, strings.HasSuffix(repo.failedMsg, "..."))
	require.NotNil(t, repo.nextAttempt)
}

func TestWorkerSkipsMissingDelivery(t *testing.T) {
	repo := &workerRepo{startAttempt: ErrDeliveryNotFound}
	worker := NewWorker(repo, signerFunc(nil), httpClientFunc(nil))

	err := worker.HandleDeliveryTask(context.Background(), taskForDelivery(t, "missing"))

	require.Error(t, err)
	assert.True(t, errors.Is(err, asynq.SkipRetry))
}

func TestBackoffDelayIsCappedExponential(t *testing.T) {
	assert.Equal(t, time.Minute, BackoffDelay(1))
	assert.Equal(t, 2*time.Minute, BackoffDelay(2))
	assert.Equal(t, 4*time.Minute, BackoffDelay(3))
	assert.Equal(t, 6*time.Hour, BackoffDelay(20))
}

func testDelivery(attempts int, maxAttempts int) *Delivery {
	return &Delivery{
		ID:             "delivery-1",
		ActivityID:     "activity-1",
		ActivityAPID:   "https://local.example/activities/1",
		ActorID:        "local-actor",
		ActorAPID:      "https://local.example/users/alice",
		TargetInboxURL: "https://remote.example/inbox",
		State:          StateProcessing,
		Attempts:       attempts,
		MaxAttempts:    maxAttempts,
		Document:       []byte(`{"id":"https://local.example/activities/1","type":"Create"}`),
	}
}

func taskForDelivery(t *testing.T, deliveryID string) *asynq.Task {
	t.Helper()

	payload, err := json.Marshal(TaskPayload{DeliveryID: deliveryID})
	require.NoError(t, err)
	return asynq.NewTask(TaskDeliver, payload)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

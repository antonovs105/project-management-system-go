package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/hibiken/asynq"
)

const (
	// deliveryUserAgent identifies outbound federation delivery HTTP requests.
	deliveryUserAgent = "project-management-system-go/activitypub-delivery"
	// maxResponseSnippetBytes bounds stored remote failure response snippets.
	maxResponseSnippetBytes = int64(1024)
	// DeliveryMetricDelivered marks a successful remote inbox delivery.
	DeliveryMetricDelivered = "delivered"
	// DeliveryMetricRetryableFailure marks a failure that will be retried.
	DeliveryMetricRetryableFailure = "retryable_failure"
	// DeliveryMetricDead marks a permanent or exhausted delivery failure.
	DeliveryMetricDead = "dead"
	// DeliveryMetricSkipped marks a task that did not need delivery work.
	DeliveryMetricSkipped = "skipped"
	// DeliveryMetricError marks local worker or repository errors.
	DeliveryMetricError = "error"
	// DeliveryMetricInvalidTask marks malformed Asynq task payloads.
	DeliveryMetricInvalidTask = "invalid_task"
	// DeliveryMetricFailureNone marks delivery outcomes without a failure category.
	DeliveryMetricFailureNone = "none"
	// DeliveryMetricFailureRepository marks local repository failures.
	DeliveryMetricFailureRepository = "repository"
	// DeliveryMetricFailureTask marks malformed or incomplete task payloads.
	DeliveryMetricFailureTask = "task"
)

// Signer signs outbound ActivityPub HTTP requests for a local actor.
type Signer interface {
	SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error
}

// HTTPClient sends outbound HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// RemoteActorRefresher refreshes cached remote actor metadata before delivery.
type RemoteActorRefresher interface {
	RefreshIfStale(ctx context.Context, actorAPID string, maxAge time.Duration) error
}

// WorkerMetrics records ActivityPub delivery worker outcomes.
type WorkerMetrics interface {
	IncDeliveryInFlight()
	DecDeliveryInFlight()
	ObserveDeliveryAttempt(outcome, failureKind string, statusCode *int, duration time.Duration)
}

// FailureNotifier receives terminal delivery failures for local user alerts.
type FailureNotifier interface {
	NotifyFederationDeliveryFailed(ctx context.Context, delivery Delivery, failureKind string) error
}

// remoteActorInboxResolver resolves a remote actor from a target inbox URL.
type remoteActorInboxResolver interface {
	RemoteActorAPIDByInboxURL(ctx context.Context, inboxURL string) (string, error)
}

// Worker executes outbound federation delivery tasks.
type Worker struct {
	repo                     Repository
	signer                   Signer
	client                   HTTPClient
	remoteActorRefresher     RemoteActorRefresher
	targetActorRefreshMaxAge time.Duration
	requireHTTPS             bool
	allowPrivateNetworks     bool
	metrics                  WorkerMetrics
	failureNotifier          FailureNotifier
}

// WorkerOption configures a delivery worker.
type WorkerOption func(*Worker)

// NewWorker creates a delivery worker with safe HTTP defaults.
func NewWorker(repo Repository, signer Signer, client HTTPClient, opts ...WorkerOption) *Worker {
	worker := &Worker{
		repo:                     repo,
		signer:                   signer,
		client:                   client,
		targetActorRefreshMaxAge: 24 * time.Hour,
	}
	for _, opt := range opts {
		opt(worker)
	}
	if worker.client == nil {
		var policy []netguard.URLPolicyOption
		if worker.requireHTTPS {
			policy = append(policy, netguard.RequireHTTPS())
		}
		if worker.allowPrivateNetworks {
			policy = append(policy, netguard.AllowPrivateNetworks())
		}
		worker.client = netguard.NewHTTPClientWithPolicy(20*time.Second, policy...)
	}
	return worker
}

// WithRemoteActorRefresher refreshes target actors before sending deliveries.
func WithRemoteActorRefresher(refresher RemoteActorRefresher) WorkerOption {
	return func(w *Worker) {
		w.remoteActorRefresher = refresher
	}
}

// WithTargetActorRefreshMaxAge sets the maximum age before a target actor refresh.
func WithTargetActorRefreshMaxAge(maxAge time.Duration) WorkerOption {
	return func(w *Worker) {
		if maxAge >= 0 {
			w.targetActorRefreshMaxAge = maxAge
		}
	}
}

// WithMetrics attaches Prometheus-compatible delivery metrics to the worker.
func WithMetrics(metrics WorkerMetrics) WorkerOption {
	return func(w *Worker) {
		w.metrics = metrics
	}
}

// WithFailureNotifier attaches terminal federation failure notification delivery.
func WithFailureNotifier(notifier FailureNotifier) WorkerOption {
	return func(w *Worker) {
		w.failureNotifier = notifier
	}
}

// WithRequireHTTPS rejects plain HTTP inbox URLs and HTTP redirects.
func WithRequireHTTPS(require bool) WorkerOption {
	return func(w *Worker) {
		w.requireHTTPS = require
	}
}

// WithAllowPrivateNetworks permits private/local network targets for isolated
// development federation tests. Do not enable it in production.
func WithAllowPrivateNetworks(allow bool) WorkerOption {
	return func(w *Worker) {
		w.allowPrivateNetworks = allow
	}
}

// NewAsynqServer creates the federation Asynq worker server.
func NewAsynqServer(redis asynq.RedisConnOpt) *asynq.Server {
	return asynq.NewServer(redis, asynq.Config{
		Concurrency:    5,
		Queues:         map[string]int{QueueFederation: 1},
		RetryDelayFunc: RetryDelay,
	})
}

// NewServeMux routes federation delivery tasks to a worker.
func NewServeMux(worker *Worker) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskDeliver, worker.HandleDeliveryTask)
	return mux
}

// HandleDeliveryTask processes one outbound delivery task.
func (w *Worker) HandleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	startedAt := time.Now()
	w.incDeliveryInFlight()
	defer w.decDeliveryInFlight()

	var payload TaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		w.observeDeliveryAttempt(DeliveryMetricInvalidTask, DeliveryMetricFailureTask, nil, startedAt)
		return fmt.Errorf("decode delivery task: %w: %w", err, asynq.SkipRetry)
	}
	if payload.DeliveryID == "" {
		w.observeDeliveryAttempt(DeliveryMetricInvalidTask, DeliveryMetricFailureTask, nil, startedAt)
		return fmt.Errorf("delivery id required: %w", asynq.SkipRetry)
	}

	delivery, err := w.repo.StartAttempt(ctx, payload.DeliveryID)
	if err != nil {
		if errors.Is(err, ErrDeliveryDone) {
			w.observeDeliveryAttempt(DeliveryMetricSkipped, DeliveryMetricFailureNone, nil, startedAt)
			return nil
		}
		if errors.Is(err, ErrDeliveryNotFound) || errors.Is(err, ErrDeliveryExhausted) {
			w.observeDeliveryAttempt(DeliveryMetricSkipped, DeliveryMetricFailureNone, nil, startedAt)
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		w.observeDeliveryAttempt(DeliveryMetricError, DeliveryMetricFailureRepository, nil, startedAt)
		return err
	}

	if err := w.deliver(ctx, delivery); err != nil {
		nextAttemptAt := time.Now().UTC().Add(BackoffDelay(delivery.Attempts))
		retryable := isRetryable(err) && delivery.Attempts < delivery.MaxAttempts
		if !retryable {
			nextAttemptAt = time.Time{}
		}

		var next *time.Time
		if !nextAttemptAt.IsZero() {
			next = &nextAttemptAt
		}
		details := failureDetailsFromError(err)
		log.Printf(
			"activitypub_delivery_failure delivery_id=%s activity_id=%s activity_ap_id=%s target_inbox_url=%s attempt=%d max_attempts=%d failure_kind=%s status_code=%s retryable=%t error=%q",
			delivery.ID,
			delivery.ActivityID,
			delivery.ActivityAPID,
			delivery.TargetInboxURL,
			delivery.Attempts,
			delivery.MaxAttempts,
			details.Kind,
			statusCodeLogValue(details.StatusCode),
			retryable,
			err.Error(),
		)
		if markErr := w.repo.MarkFailed(ctx, delivery.ID, err.Error(), details, next); markErr != nil {
			w.observeDeliveryAttempt(DeliveryMetricError, DeliveryMetricFailureRepository, nil, startedAt)
			return markErr
		}
		if !retryable {
			if w.failureNotifier != nil {
				if notifyErr := w.failureNotifier.NotifyFederationDeliveryFailed(ctx, *delivery, details.Kind); notifyErr != nil {
					log.Printf("activitypub_delivery_notification_failed delivery_id=%s error=%q", delivery.ID, notifyErr.Error())
				}
			}
			w.observeDeliveryAttempt(DeliveryMetricDead, details.Kind, details.StatusCode, startedAt)
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		w.observeDeliveryAttempt(DeliveryMetricRetryableFailure, details.Kind, details.StatusCode, startedAt)
		return err
	}

	if err := w.repo.MarkDelivered(ctx, delivery.ID); err != nil {
		w.observeDeliveryAttempt(DeliveryMetricError, DeliveryMetricFailureRepository, nil, startedAt)
		return err
	}
	log.Printf(
		"activitypub_delivery_success delivery_id=%s activity_id=%s activity_ap_id=%s target_inbox_url=%s attempt=%d",
		delivery.ID,
		delivery.ActivityID,
		delivery.ActivityAPID,
		delivery.TargetInboxURL,
		delivery.Attempts,
	)
	w.observeDeliveryAttempt(DeliveryMetricDelivered, DeliveryMetricFailureNone, nil, startedAt)
	return nil
}

// incDeliveryInFlight increments worker in-flight metrics when configured.
func (w *Worker) incDeliveryInFlight() {
	if w.metrics != nil {
		w.metrics.IncDeliveryInFlight()
	}
}

// decDeliveryInFlight decrements worker in-flight metrics when configured.
func (w *Worker) decDeliveryInFlight() {
	if w.metrics != nil {
		w.metrics.DecDeliveryInFlight()
	}
}

// observeDeliveryAttempt records one worker outcome when metrics are configured.
func (w *Worker) observeDeliveryAttempt(outcome, failureKind string, statusCode *int, startedAt time.Time) {
	if w.metrics != nil {
		w.metrics.ObserveDeliveryAttempt(outcome, failureKind, statusCode, time.Since(startedAt))
	}
}

// deliver signs and POSTs one stored ActivityPub activity to a remote inbox.
func (w *Worker) deliver(ctx context.Context, delivery *Delivery) error {
	if err := validateTargetInboxURL(delivery.TargetInboxURL, w.requireHTTPS); err != nil {
		return err
	}
	w.refreshTargetActor(ctx, delivery.TargetInboxURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.TargetInboxURL, bytes.NewReader(delivery.Document))
	if err != nil {
		return permanentError{err: err}
	}
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("User-Agent", deliveryUserAgent)

	if err := w.signer.SignRequest(ctx, delivery.ActorID, req, delivery.Document); err != nil {
		if errors.Is(err, httpsig.ErrInvalidPrivateKey) || errors.Is(err, httpsig.ErrUnsupportedAlgorithm) {
			return permanentError{err: err}
		}
		return err
	}

	resp, err := w.client.Do(req)
	if err != nil {
		if errors.Is(err, netguard.ErrUnsafeURL) || errors.Is(err, netguard.ErrTooManyRedirects) {
			return permanentError{err: err}
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	message := responseMessage(resp)
	if isRetryableStatus(resp.StatusCode) {
		return httpStatusError{statusCode: resp.StatusCode, err: errors.New(message)}
	}
	return permanentError{err: httpStatusError{statusCode: resp.StatusCode, err: errors.New(message)}}
}

// refreshTargetActor refreshes cached metadata for the inbox owner when possible.
func (w *Worker) refreshTargetActor(ctx context.Context, inboxURL string) {
	if w.remoteActorRefresher == nil {
		return
	}
	resolver, ok := w.repo.(remoteActorInboxResolver)
	if !ok {
		return
	}
	actorAPID, err := resolver.RemoteActorAPIDByInboxURL(ctx, inboxURL)
	if err != nil || actorAPID == "" {
		return
	}
	_ = w.remoteActorRefresher.RefreshIfStale(ctx, actorAPID, w.targetActorRefreshMaxAge)
}

// responseMessage captures a bounded response body snippet for delivery failures.
func responseMessage(resp *http.Response) string {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSnippetBytes+1))
	if len(raw) == 0 {
		return fmt.Sprintf("delivery failed: %s", resp.Status)
	}
	truncated := int64(len(raw)) > maxResponseSnippetBytes
	if truncated {
		raw = raw[:maxResponseSnippetBytes]
	}
	snippet := sanitizeResponseSnippet(string(raw))
	if truncated {
		snippet += "..."
	}
	return fmt.Sprintf("delivery failed: %s: %s", resp.Status, snippet)
}

// RetryDelay maps Asynq retry count to the delivery exponential backoff delay.
func RetryDelay(n int, err error, task *asynq.Task) time.Duration {
	return BackoffDelay(n + 1)
}

// BackoffDelay returns capped exponential backoff for a one-based attempt number.
func BackoffDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Minute
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= 6*time.Hour {
			return 6 * time.Hour
		}
	}
	return delay
}

// isRetryableStatus reports whether an HTTP status should be retried.
func isRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

// isRetryable reports whether an error should consume another worker retry attempt.
func isRetryable(err error) bool {
	var permanent permanentError
	return !errors.As(err, &permanent)
}

// failureDetailsFromError extracts structured failure metadata from delivery errors.
func failureDetailsFromError(err error) FailureDetails {
	var httpErr httpStatusError
	if errors.As(err, &httpErr) {
		statusCode := httpErr.statusCode
		return FailureDetails{Kind: FailureKindHTTP, StatusCode: &statusCode}
	}
	switch {
	case errors.Is(err, httpsig.ErrInvalidPrivateKey), errors.Is(err, httpsig.ErrUnsupportedAlgorithm):
		return FailureDetails{Kind: FailureKindSigning}
	case errors.Is(err, netguard.ErrUnsafeURL), errors.Is(err, netguard.ErrTooManyRedirects):
		return FailureDetails{Kind: FailureKindSafety}
	case isRetryable(err):
		return FailureDetails{Kind: FailureKindNetwork}
	default:
		return FailureDetails{Kind: FailureKindUnknown}
	}
}

// statusCodeLogValue formats optional HTTP status codes for logs.
func statusCodeLogValue(statusCode *int) string {
	if statusCode == nil {
		return ""
	}
	return fmt.Sprintf("%d", *statusCode)
}

// validateTargetInboxURL rejects unsafe target inbox URLs before delivery.
func validateTargetInboxURL(raw string, requireHTTPS bool) error {
	var policy []netguard.URLPolicyOption
	if requireHTTPS {
		policy = append(policy, netguard.RequireHTTPS())
	}
	if _, err := netguard.ValidateRemoteURL(raw, policy...); err != nil {
		return permanentError{err: err}
	}
	return nil
}

// sanitizeResponseSnippet strips control characters from remote error snippets.
func sanitizeResponseSnippet(raw string) string {
	raw = strings.ToValidUTF8(raw, "")
	raw = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)
	return strings.TrimSpace(raw)
}

// permanentError marks a delivery failure that should not be retried.
type permanentError struct {
	err error
}

// Error returns the wrapped permanent error message.
func (e permanentError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying permanent error.
func (e permanentError) Unwrap() error {
	return e.err
}

// httpStatusError carries a non-success remote response status.
type httpStatusError struct {
	statusCode int
	err        error
}

// Error returns the HTTP status failure message.
func (e httpStatusError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying HTTP status error.
func (e httpStatusError) Unwrap() error {
	return e.err
}

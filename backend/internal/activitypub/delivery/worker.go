package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/hibiken/asynq"
)

const (
	deliveryUserAgent       = "project-management-system-go/activitypub-delivery"
	maxResponseSnippetBytes = int64(1024)
)

type Signer interface {
	SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Worker struct {
	repo   Repository
	signer Signer
	client HTTPClient
}

func NewWorker(repo Repository, signer Signer, client HTTPClient) *Worker {
	if client == nil {
		client = netguard.NewHTTPClient(20 * time.Second)
	}
	return &Worker{repo: repo, signer: signer, client: client}
}

func NewAsynqServer(redis asynq.RedisConnOpt) *asynq.Server {
	return asynq.NewServer(redis, asynq.Config{
		Concurrency:    5,
		Queues:         map[string]int{QueueFederation: 1},
		RetryDelayFunc: RetryDelay,
	})
}

func NewServeMux(worker *Worker) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskDeliver, worker.HandleDeliveryTask)
	return mux
}

func (w *Worker) HandleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	var payload TaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode delivery task: %w: %w", err, asynq.SkipRetry)
	}
	if payload.DeliveryID == "" {
		return fmt.Errorf("delivery id required: %w", asynq.SkipRetry)
	}

	delivery, err := w.repo.StartAttempt(ctx, payload.DeliveryID)
	if err != nil {
		if errors.Is(err, ErrDeliveryDone) {
			return nil
		}
		if errors.Is(err, ErrDeliveryNotFound) || errors.Is(err, ErrDeliveryExhausted) {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
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
		if markErr := w.repo.MarkFailed(ctx, delivery.ID, err.Error(), next); markErr != nil {
			return markErr
		}
		if !retryable {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
		return err
	}

	return w.repo.MarkDelivered(ctx, delivery.ID)
}

func (w *Worker) deliver(ctx context.Context, delivery *Delivery) error {
	if err := validateTargetInboxURL(delivery.TargetInboxURL); err != nil {
		return err
	}

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
		return errors.New(message)
	}
	return permanentError{err: errors.New(message)}
}

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

func RetryDelay(n int, err error, task *asynq.Task) time.Duration {
	return BackoffDelay(n + 1)
}

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

func isRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func isRetryable(err error) bool {
	var permanent permanentError
	return !errors.As(err, &permanent)
}

func validateTargetInboxURL(raw string) error {
	if _, err := netguard.ValidateRemoteURL(raw); err != nil {
		return permanentError{err: err}
	}
	return nil
}

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

type permanentError struct {
	err error
}

func (e permanentError) Error() string {
	return e.err.Error()
}

func (e permanentError) Unwrap() error {
	return e.err
}

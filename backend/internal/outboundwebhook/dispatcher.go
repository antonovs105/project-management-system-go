package outboundwebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
)

// DispatchRepository is the durable delivery queue contract.
type DispatchRepository interface {
	EnqueueActivityEvents(context.Context) (int, error)
	Claim(context.Context, time.Time) (*Delivery, error)
	Complete(context.Context, string, int, time.Time) error
	Fail(context.Context, *Delivery, *int, string, time.Time) error
}

// DispatcherOption configures outbound HTTP safety.
type DispatcherOption func(*Dispatcher)

// WithDispatcherRequireHTTPS requires HTTPS delivery targets.
func WithDispatcherRequireHTTPS(required bool) DispatcherOption {
	return func(d *Dispatcher) { d.requireHTTPS = required }
}

// WithDispatcherAllowPrivateNetworks permits private targets in isolated development.
func WithDispatcherAllowPrivateNetworks(allow bool) DispatcherOption {
	return func(d *Dispatcher) { d.allowPrivateNetworks = allow }
}

// WithHTTPClient injects a test or instrumented HTTP client.
func WithHTTPClient(client *http.Client) DispatcherOption {
	return func(d *Dispatcher) { d.client = client }
}

// Dispatcher signs and retries durable webhook deliveries.
type Dispatcher struct {
	repository           DispatchRepository
	secretCodec          secrets.PrivateKeyCodec
	client               *http.Client
	now                  func() time.Time
	requireHTTPS         bool
	allowPrivateNetworks bool
}

// NewDispatcher creates an outbound webhook dispatcher.
func NewDispatcher(repository DispatchRepository, secretCodec secrets.PrivateKeyCodec, options ...DispatcherOption) *Dispatcher {
	dispatcher := &Dispatcher{repository: repository, secretCodec: secretCodec, now: time.Now}
	for _, option := range options {
		option(dispatcher)
	}
	if dispatcher.client == nil {
		dispatcher.client = netguard.NewHTTPClientWithPolicy(10*time.Second, dispatcher.urlPolicy()...)
	}
	return dispatcher
}

// DispatchOnce materializes and processes up to limit due deliveries.
func (d *Dispatcher) DispatchOnce(ctx context.Context, limit int) (int, error) {
	if _, err := d.repository.EnqueueActivityEvents(ctx); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	processed := 0
	for processed < limit {
		delivery, err := d.repository.Claim(ctx, d.now().UTC())
		if errors.Is(err, ErrNotFound) {
			return processed, nil
		}
		if err != nil {
			return processed, err
		}
		processed++
		if err := d.dispatch(ctx, delivery); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

// StartLoop runs bounded delivery batches until the returned stop function is called.
func (d *Dispatcher) StartLoop(ctx context.Context, interval time.Duration, report func(int, error)) func() {
	loopCtx, cancel := context.WithCancel(ctx)
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			processed, err := d.DispatchOnce(loopCtx, 100)
			if report != nil && (processed > 0 || err != nil) {
				report(processed, err)
			}
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

func (d *Dispatcher) dispatch(ctx context.Context, delivery *Delivery) error {
	secret, err := d.secretCodec.DecryptPrivateKey(delivery.SecretCipher)
	if err != nil {
		return d.fail(ctx, delivery, nil, "decrypt signing secret: "+err.Error())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.TargetURL, bytes.NewReader(delivery.Payload))
	if err != nil {
		return d.fail(ctx, delivery, nil, err.Error())
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(delivery.Payload)
	request.Header.Set(echoContentType, "application/json")
	request.Header.Set("User-Agent", "Progo-Webhook/1.0")
	request.Header.Set("X-Progo-Event", delivery.EventType)
	request.Header.Set("X-Progo-Delivery", delivery.ID)
	request.Header.Set("X-Progo-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response, err := d.client.Do(request)
	if err != nil {
		return d.fail(ctx, delivery, nil, err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := response.StatusCode
		return d.fail(ctx, delivery, &status, fmt.Sprintf("webhook returned HTTP %d", status))
	}
	return d.repository.Complete(ctx, delivery.ID, response.StatusCode, d.now().UTC())
}

func (d *Dispatcher) fail(ctx context.Context, delivery *Delivery, statusCode *int, failure string) error {
	delay := time.Duration(1<<min(delivery.Attempts, 6)) * time.Minute
	return d.repository.Fail(ctx, delivery, statusCode, failure, d.now().UTC().Add(delay))
}

func (d *Dispatcher) urlPolicy() []netguard.URLPolicyOption {
	var policy []netguard.URLPolicyOption
	if d.requireHTTPS {
		policy = append(policy, netguard.RequireHTTPS())
	}
	if d.allowPrivateNetworks {
		policy = append(policy, netguard.AllowPrivateNetworks())
	}
	return policy
}

const echoContentType = "Content-Type"

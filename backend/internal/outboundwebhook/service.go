package outboundwebhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
	"github.com/antonovs105/project-management-system-go/internal/apperror"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/google/uuid"
)

// ErrInvalidInput reports malformed outbound webhook configuration.
var ErrInvalidInput = errors.New("invalid outbound webhook input")

// ProjectAuthorizer checks project-level integration management permission.
type ProjectAuthorizer interface {
	HasProjectPermission(context.Context, string, string, string) (bool, error)
}

// ServiceRepository is the configuration and diagnostic persistence contract.
type ServiceRepository interface {
	Create(context.Context, *Webhook, string) error
	List(context.Context, string) ([]Webhook, error)
	Delete(context.Context, string, string) error
	ListDeliveries(context.Context, string, int) ([]Delivery, error)
	Retry(context.Context, string, string) error
}

// ServiceOption configures outbound webhook validation.
type ServiceOption func(*Service)

// WithRequireHTTPS requires HTTPS callback targets.
func WithRequireHTTPS(required bool) ServiceOption {
	return func(s *Service) { s.requireHTTPS = required }
}

// WithAllowPrivateNetworks permits loopback/private callbacks for isolated development.
func WithAllowPrivateNetworks(allow bool) ServiceOption {
	return func(s *Service) { s.allowPrivateNetworks = allow }
}

// Service manages project webhook configuration and diagnostics.
type Service struct {
	repository           ServiceRepository
	projects             ProjectAuthorizer
	secretCodec          secrets.PrivateKeyCodec
	requireHTTPS         bool
	allowPrivateNetworks bool
}

// NewService creates an outbound webhook service.
func NewService(repository ServiceRepository, projects ProjectAuthorizer, secretCodec secrets.PrivateKeyCodec, options ...ServiceOption) *Service {
	service := &Service{repository: repository, projects: projects, secretCodec: secretCodec}
	for _, option := range options {
		option(service)
	}
	return service
}

// Create validates and stores a signed project callback.
func (s *Service) Create(ctx context.Context, projectID, userID, name, targetURL string, events []string) (*CreatedWebhook, error) {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	targetURL = strings.TrimSpace(targetURL)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return nil, invalidWebhookInput("name must contain 1 to 80 characters")
	}
	policy := s.urlPolicy()
	parsed, err := netguard.ValidateRemoteURL(targetURL, policy...)
	if err != nil {
		return nil, invalidWebhookInput("target URL is not allowed")
	}
	events, err = normalizeEvents(events)
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := "whsec_" + base64.RawURLEncoding.EncodeToString(raw)
	ciphertext, err := s.secretCodec.EncryptPrivateKey(secret)
	if err != nil {
		return nil, err
	}
	value := Webhook{ProjectID: projectID, CreatedBy: userID, Name: name, TargetURL: parsed.String(), Events: events}
	if err := s.repository.Create(ctx, &value, ciphertext); err != nil {
		return nil, err
	}
	return &CreatedWebhook{Webhook: value, Secret: secret}, nil
}

// List returns project webhook metadata.
func (s *Service) List(ctx context.Context, projectID, userID string) ([]Webhook, error) {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, projectID)
}

// Delete removes a project webhook.
func (s *Service) Delete(ctx context.Context, projectID, webhookID, userID string) error {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return err
	}
	if _, err := uuid.Parse(webhookID); err != nil {
		return invalidWebhookInput("valid webhook id is required")
	}
	return s.repository.Delete(ctx, projectID, webhookID)
}

// ListDeliveries returns recent delivery diagnostics.
func (s *Service) ListDeliveries(ctx context.Context, projectID, userID string) ([]Delivery, error) {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return nil, err
	}
	return s.repository.ListDeliveries(ctx, projectID, 100)
}

// Retry reschedules a terminal or failed delivery.
func (s *Service) Retry(ctx context.Context, projectID, deliveryID, userID string) error {
	if err := s.requireManage(ctx, projectID, userID); err != nil {
		return err
	}
	if _, err := uuid.Parse(deliveryID); err != nil {
		return invalidWebhookInput("valid delivery id is required")
	}
	return s.repository.Retry(ctx, projectID, deliveryID)
}

func (s *Service) requireManage(ctx context.Context, projectID, userID string) error {
	if _, err := uuid.Parse(projectID); err != nil {
		return invalidWebhookInput("valid project id is required")
	}
	allowed, err := s.projects.HasProjectPermission(ctx, projectID, userID, project.PermissionProjectUpdate)
	if err != nil {
		return apperror.New(apperror.ErrNotFound, "project not found or access denied")
	}
	if !allowed {
		return apperror.New(apperror.ErrForbidden, "insufficient permissions: missing project.update")
	}
	return nil
}

func (s *Service) urlPolicy() []netguard.URLPolicyOption {
	var policy []netguard.URLPolicyOption
	if s.requireHTTPS {
		policy = append(policy, netguard.RequireHTTPS())
	}
	if s.allowPrivateNetworks {
		policy = append(policy, netguard.AllowPrivateNetworks())
	}
	return policy
}

func normalizeEvents(events []string) ([]string, error) {
	allowed := make(map[string]struct{}, len(SupportedEvents))
	for _, event := range SupportedEvents {
		allowed[event] = struct{}{}
	}
	unique := make(map[string]struct{}, len(events))
	for _, event := range events {
		event = strings.ToLower(strings.TrimSpace(event))
		if _, ok := allowed[event]; !ok {
			return nil, invalidWebhookInput(fmt.Sprintf("unsupported event %q", event))
		}
		unique[event] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, invalidWebhookInput("at least one event is required")
	}
	result := make([]string, 0, len(unique))
	for event := range unique {
		result = append(result, event)
	}
	sort.Strings(result)
	return result, nil
}

func invalidWebhookInput(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }

package remoteactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
)

const (
	defaultWebFingerScheme = "https"
	defaultUserAgent       = "project-management-system-go/activitypub"
	defaultMaxResponseSize = int64(2 << 20)
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Service struct {
	repo            Repository
	client          HTTPClient
	webFingerScheme string
	userAgent       string
	maxResponseSize int64
}

type Option func(*Service)

func NewService(repo Repository, opts ...Option) *Service {
	service := &Service{
		repo:            repo,
		client:          &http.Client{Timeout: 10 * time.Second},
		webFingerScheme: defaultWebFingerScheme,
		userAgent:       defaultUserAgent,
		maxResponseSize: defaultMaxResponseSize,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func WithHTTPClient(client HTTPClient) Option {
	return func(s *Service) {
		if client != nil {
			s.client = client
		}
	}
}

func WithWebFingerScheme(scheme string) Option {
	return func(s *Service) {
		scheme = strings.ToLower(strings.TrimSpace(scheme))
		if scheme == "http" || scheme == "https" {
			s.webFingerScheme = scheme
		}
	}
}

func WithMaxResponseSize(size int64) Option {
	return func(s *Service) {
		if size > 0 {
			s.maxResponseSize = size
		}
	}
}

func (s *Service) Discover(ctx context.Context, resource string) (*Actor, error) {
	username, domain, normalizedResource, err := normalizeAcctResource(resource)
	if err != nil {
		return nil, err
	}

	actorURL, err := s.resolveWebFinger(ctx, domain, normalizedResource)
	if err != nil {
		return nil, err
	}

	actor, err := s.fetchActor(ctx, actorURL, username, domain)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpsertRemoteActor(ctx, actor); err != nil {
		return nil, err
	}
	return actor, nil
}

func (s *Service) resolveWebFinger(ctx context.Context, domain, resource string) (string, error) {
	values := url.Values{}
	values.Set("resource", resource)
	endpoint := url.URL{
		Scheme:   s.webFingerScheme,
		Host:     domain,
		Path:     "/.well-known/webfinger",
		RawQuery: values.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/jrd+json")
	req.Header.Set("User-Agent", s.userAgent)

	var doc webFingerDocument
	if err := s.getJSON(req, &doc); err != nil {
		return "", err
	}
	for _, link := range doc.Links {
		if link.Rel != "self" {
			continue
		}
		if link.Type != "" && !isActivityJSONMediaType(link.Type) {
			continue
		}
		if link.Href == "" {
			return "", ErrInvalidWebFinger
		}
		return link.Href, nil
	}
	return "", ErrInvalidWebFinger
}

func (s *Service) fetchActor(ctx context.Context, actorURL, fallbackUsername, domain string) (*Actor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	req.Header.Set("User-Agent", s.userAgent)

	raw, err := s.getRaw(req)
	if err != nil {
		return nil, err
	}

	var doc actorDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidActorDocument, err)
	}

	actorType, err := normalizeActorType(doc.Type)
	if err != nil {
		return nil, err
	}
	if doc.ID == "" || doc.ID != actorURL {
		return nil, fmt.Errorf("%w: actor id mismatch", ErrInvalidActorDocument)
	}
	if doc.Inbox == "" || doc.Outbox == "" {
		return nil, fmt.Errorf("%w: actor inbox/outbox required", ErrInvalidActorDocument)
	}

	keyID, publicKeyPEM, err := parsePublicKey(doc.PublicKey, doc.ID)
	if err != nil {
		return nil, err
	}

	preferredUsername := strings.TrimSpace(doc.PreferredUsername)
	if preferredUsername == "" {
		preferredUsername = fallbackUsername
	}
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		name = preferredUsername
	}
	handle := fallbackUsername + "@" + strings.ToLower(domain)

	return &Actor{
		APID:              doc.ID,
		Type:              actorType,
		PreferredUsername: preferredUsername,
		Handle:            handle,
		Name:              name,
		Summary:           doc.Summary,
		InboxURL:          doc.Inbox,
		OutboxURL:         doc.Outbox,
		FollowersURL:      emptyStringToNil(doc.Followers),
		FollowingURL:      emptyStringToNil(doc.Following),
		PublicKeyID:       keyID,
		PublicKeyPEM:      publicKeyPEM,
		Document:          append([]byte(nil), raw...),
	}, nil
}

func (s *Service) getJSON(req *http.Request, target any) error {
	raw, err := s.getRaw(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidWebFinger, err)
	}
	return nil
}

func (s *Service) getRaw(req *http.Request) ([]byte, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, resp.Status)
	}

	var buf bytes.Buffer
	limited := io.LimitReader(resp.Body, s.maxResponseSize+1)
	if _, err := buf.ReadFrom(limited); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > s.maxResponseSize {
		return nil, fmt.Errorf("%w: response too large", ErrInvalidActorDocument)
	}
	return buf.Bytes(), nil
}

func normalizeAcctResource(resource string) (username string, domain string, normalized string, err error) {
	resource = strings.TrimSpace(resource)
	if resource == "" || !strings.HasPrefix(strings.ToLower(resource), "acct:") {
		return "", "", "", ErrInvalidResource
	}
	account := resource[len("acct:"):]
	username, domain, ok := strings.Cut(account, "@")
	if !ok || username == "" || domain == "" {
		return "", "", "", ErrInvalidResource
	}
	if strings.Contains(username, "/") || strings.ContainsAny(username, " \t\r\n") {
		return "", "", "", ErrInvalidResource
	}
	if strings.ContainsAny(domain, " \t\r\n/") {
		return "", "", "", ErrInvalidResource
	}
	domain = strings.ToLower(domain)
	return username, domain, "acct:" + username + "@" + domain, nil
}

func isActivityJSONMediaType(raw string) bool {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(raw, ";")[0])
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/activity+json" || mediaType == "application/ld+json"
}

func normalizeActorType(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		if isSupportedActorType(typed) {
			return typed, nil
		}
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok && isSupportedActorType(value) {
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("%w: unsupported actor type", ErrInvalidActorDocument)
}

func isSupportedActorType(value string) bool {
	switch value {
	case "Person", "Group", "Organization", "Application", "Service":
		return true
	default:
		return false
	}
}

func parsePublicKey(raw json.RawMessage, actorID string) (keyID string, publicKeyPEM string, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", fmt.Errorf("%w: publicKey required", ErrInvalidActorDocument)
	}

	var key publicKeyDocument
	if err := json.Unmarshal(raw, &key); err != nil {
		return "", "", fmt.Errorf("%w: publicKey object required", ErrInvalidActorDocument)
	}
	if key.ID == "" || key.PublicKeyPEM == "" {
		return "", "", fmt.Errorf("%w: publicKey id and pem required", ErrInvalidActorDocument)
	}
	if key.Owner != "" && key.Owner != actorID {
		return "", "", fmt.Errorf("%w: publicKey owner mismatch", ErrInvalidActorDocument)
	}
	if _, err := httpsig.ParseRSAPublicKeyPEM(key.PublicKeyPEM); err != nil {
		return "", "", fmt.Errorf("%w: invalid publicKeyPem", ErrInvalidActorDocument)
	}
	return key.ID, key.PublicKeyPEM, nil
}

func emptyStringToNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

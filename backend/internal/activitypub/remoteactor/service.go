package remoteactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/netguard"
)

const (
	// defaultWebFingerScheme is the default scheme for remote WebFinger discovery.
	defaultWebFingerScheme = "https"
	// defaultUserAgent identifies remote actor discovery requests.
	defaultUserAgent = "project-management-system-go/activitypub"
	// defaultMaxResponseSize bounds remote WebFinger and actor response bodies.
	defaultMaxResponseSize = int64(2 << 20)
)

// HTTPClient sends remote actor discovery and fetch requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Service discovers, fetches, validates, and caches remote ActivityPub actors.
type Service struct {
	repo            Repository
	client          HTTPClient
	customClient    bool
	webFingerScheme string
	userAgent       string
	maxResponseSize int64
	requireHTTPS    bool
}

// Option configures a remote actor service.
type Option func(*Service)

// NewService creates a remote actor discovery service.
func NewService(repo Repository, opts ...Option) *Service {
	service := &Service{
		repo:            repo,
		client:          netguard.NewHTTPClient(10 * time.Second),
		webFingerScheme: defaultWebFingerScheme,
		userAgent:       defaultUserAgent,
		maxResponseSize: defaultMaxResponseSize,
	}
	for _, opt := range opts {
		opt(service)
	}
	if service.requireHTTPS && !service.customClient {
		service.client = netguard.NewHTTPClientWithPolicy(10*time.Second, netguard.RequireHTTPS())
	}
	return service
}

// WithHTTPClient overrides the safe default HTTP client.
func WithHTTPClient(client HTTPClient) Option {
	return func(s *Service) {
		if client != nil {
			s.client = client
			s.customClient = true
		}
	}
}

// WithWebFingerScheme sets the scheme used for WebFinger discovery.
func WithWebFingerScheme(scheme string) Option {
	return func(s *Service) {
		scheme = strings.ToLower(strings.TrimSpace(scheme))
		if scheme == "http" || scheme == "https" {
			s.webFingerScheme = scheme
		}
	}
}

// WithMaxResponseSize limits remote WebFinger and actor document response bodies.
func WithMaxResponseSize(size int64) Option {
	return func(s *Service) {
		if size > 0 {
			s.maxResponseSize = size
		}
	}
}

// WithRequireHTTPS rejects plain HTTP actor, key, endpoint, and redirect URLs.
func WithRequireHTTPS(require bool) Option {
	return func(s *Service) {
		s.requireHTTPS = require
	}
}

// Discover resolves an acct: resource through WebFinger and caches the actor.
func (s *Service) Discover(ctx context.Context, resource string) (*Actor, error) {
	username, domain, normalizedResource, err := normalizeAcctResource(resource)
	if err != nil {
		return nil, err
	}

	actorURL, err := s.resolveWebFinger(ctx, domain, normalizedResource)
	if err != nil {
		return nil, err
	}

	return s.fetchAndCacheActor(ctx, actorURL, username, domain)
}

// Fetch retrieves and caches a remote actor document by URL.
func (s *Service) Fetch(ctx context.Context, actorURL string) (*Actor, error) {
	fallbackUsername, domain, err := fallbackIdentity(actorURL)
	if err != nil {
		return nil, err
	}
	return s.fetchAndCacheActor(ctx, actorURL, fallbackUsername, domain)
}

// RefreshIfStale refreshes a cached remote actor when its cache age exceeds maxAge.
func (s *Service) RefreshIfStale(ctx context.Context, actorAPID string, maxAge time.Duration) error {
	actor, err := s.repo.RemoteActorByAPID(ctx, actorAPID)
	if err != nil {
		return err
	}
	if maxAge > 0 && actor.LastFetchedAt != nil && time.Since(actor.LastFetchedAt.UTC()) < maxAge {
		return nil
	}
	_, err = s.Fetch(ctx, actor.APID)
	return err
}

// ResolveKey fetches the actor document that owns a missing key ID.
func (s *Service) ResolveKey(ctx context.Context, keyID string) error {
	return s.fetchAndCacheKey(ctx, keyID, "")
}

// RefreshKey refreshes a known actor key and verifies the expected actor ID.
func (s *Service) RefreshKey(ctx context.Context, keyID, expectedActorAPID string) error {
	return s.fetchAndCacheKey(ctx, keyID, expectedActorAPID)
}

// fetchAndCacheKey refreshes the actor document that owns a signing key.
func (s *Service) fetchAndCacheKey(ctx context.Context, keyID, expectedActorAPID string) error {
	actorURL, err := actorURLFromKeyID(keyID, s.requireHTTPS)
	if err != nil {
		return err
	}
	if expectedActorAPID != "" {
		actorURL = expectedActorAPID
	}

	fallbackUsername, domain, err := fallbackIdentity(actorURL)
	if err != nil {
		return err
	}
	actor, err := s.fetchActor(ctx, actorURL, fallbackUsername, domain)
	if err != nil {
		s.recordFetchFailure(ctx, actorURL, err)
		return err
	}
	if actor.PublicKeyID != keyID {
		err := fmt.Errorf("%w: key id mismatch", ErrInvalidActorDocument)
		s.recordFetchFailure(ctx, actor.APID, err)
		return err
	}
	if expectedActorAPID != "" && actor.APID != expectedActorAPID {
		err := fmt.Errorf("%w: actor id changed", ErrInvalidActorDocument)
		s.recordFetchFailure(ctx, actor.APID, err)
		return err
	}
	return s.repo.UpsertRemoteActor(ctx, actor)
}

// fetchAndCacheActor fetches a remote actor document and stores its cache projection.
func (s *Service) fetchAndCacheActor(ctx context.Context, actorURL, fallbackUsername, domain string) (*Actor, error) {
	actor, err := s.fetchActor(ctx, actorURL, fallbackUsername, domain)
	if err != nil {
		s.recordFetchFailure(ctx, actorURL, err)
		return nil, err
	}
	if err := s.repo.UpsertRemoteActor(ctx, actor); err != nil {
		return nil, err
	}
	return actor, nil
}

// recordFetchFailure stores remote actor fetch failures without masking root errors.
func (s *Service) recordFetchFailure(ctx context.Context, actorURL string, err error) {
	if actorURL == "" || err == nil {
		return
	}
	log.Printf("activitypub_remote_actor_fetch_failure actor_ap_id=%s error=%q", actorURL, err.Error())
	_ = s.repo.RecordRemoteActorFetchFailure(ctx, actorURL, err.Error())
}

// resolveWebFinger resolves an acct: resource to an ActivityPub actor URL.
func (s *Service) resolveWebFinger(ctx context.Context, domain, resource string) (string, error) {
	values := url.Values{}
	values.Set("resource", resource)
	endpoint := url.URL{
		Scheme:   s.webFingerScheme,
		Host:     domain,
		Path:     "/.well-known/webfinger",
		RawQuery: values.Encode(),
	}
	if s.requireHTTPS && endpoint.Scheme != "https" {
		return "", fmt.Errorf("%w: webfinger https required", ErrInvalidWebFinger)
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

// fetchActor loads and validates a remote actor ActivityPub document.
func (s *Service) fetchActor(ctx context.Context, actorURL, fallbackUsername, domain string) (*Actor, error) {
	actorURL = strings.TrimSpace(actorURL)
	if _, err := parseHTTPURL(actorURL, s.requireHTTPS); err != nil {
		return nil, fmt.Errorf("%w: actor url", ErrInvalidActorDocument)
	}

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
	if _, err := parseHTTPURL(doc.Inbox, s.requireHTTPS); err != nil {
		return nil, fmt.Errorf("%w: actor inbox url", ErrInvalidActorDocument)
	}
	if _, err := parseHTTPURL(doc.Outbox, s.requireHTTPS); err != nil {
		return nil, fmt.Errorf("%w: actor outbox url", ErrInvalidActorDocument)
	}
	if doc.Followers != "" {
		if _, err := parseHTTPURL(doc.Followers, s.requireHTTPS); err != nil {
			return nil, fmt.Errorf("%w: actor followers url", ErrInvalidActorDocument)
		}
	}
	if doc.Following != "" {
		if _, err := parseHTTPURL(doc.Following, s.requireHTTPS); err != nil {
			return nil, fmt.Errorf("%w: actor following url", ErrInvalidActorDocument)
		}
	}

	keyID, publicKeyPEM, err := parsePublicKey(doc.PublicKey, doc.ID, s.requireHTTPS)
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

// getJSON fetches a remote JSON document into target.
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

// getRaw fetches a bounded remote response body and validates the status code.
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

// normalizeAcctResource parses and canonicalizes an acct: WebFinger resource.
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

// actorURLFromKeyID derives the actor document URL from a public key fragment URL.
func actorURLFromKeyID(keyID string, requireHTTPS bool) (string, error) {
	parsed, err := parseHTTPURL(keyID, requireHTTPS)
	if err != nil {
		return "", ErrInvalidActorDocument
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		return "", ErrInvalidActorDocument
	}
	return parsed.String(), nil
}

// fallbackIdentity derives a best-effort username and domain from an actor URL.
func fallbackIdentity(actorURL string) (username string, domain string, err error) {
	parsed, err := parseHTTPURL(actorURL, false)
	if err != nil {
		return "", "", ErrInvalidActorDocument
	}
	username = path.Base(strings.TrimRight(parsed.Path, "/"))
	if username == "." || username == "/" || username == "" {
		username = "remote"
	}
	return username, strings.ToLower(parsed.Host), nil
}

// isActivityJSONMediaType reports whether a media type can describe an actor document.
func isActivityJSONMediaType(raw string) bool {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(raw, ";")[0])
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/activity+json" || mediaType == "application/ld+json"
}

// normalizeActorType extracts a supported ActivityStreams actor type.
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

// isSupportedActorType reports whether an actor type is accepted for federation.
func isSupportedActorType(value string) bool {
	switch value {
	case "Person", "Group", "Organization", "Application", "Service":
		return true
	default:
		return false
	}
}

// parsePublicKey validates the embedded ActivityPub publicKey object.
func parsePublicKey(raw json.RawMessage, actorID string, requireHTTPS bool) (keyID string, publicKeyPEM string, err error) {
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
	if _, err := parseHTTPURL(key.ID, requireHTTPS); err != nil {
		return "", "", fmt.Errorf("%w: publicKey id url", ErrInvalidActorDocument)
	}
	if key.Owner != "" && key.Owner != actorID {
		return "", "", fmt.Errorf("%w: publicKey owner mismatch", ErrInvalidActorDocument)
	}
	if _, err := httpsig.ParseRSAPublicKeyPEM(key.PublicKeyPEM); err != nil {
		return "", "", fmt.Errorf("%w: invalid publicKeyPem", ErrInvalidActorDocument)
	}
	return key.ID, key.PublicKeyPEM, nil
}

// parseHTTPURL parses an absolute HTTP(S) URL with a path.
func parseHTTPURL(value string, requireHTTPS bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, ErrInvalidActorDocument
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidActorDocument
	}
	if requireHTTPS && scheme != "https" {
		return nil, ErrInvalidActorDocument
	}
	if parsed.Path == "" {
		return nil, ErrInvalidActorDocument
	}
	return parsed, nil
}

// emptyStringToNil converts empty strings to nil pointers for optional URLs.
func emptyStringToNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

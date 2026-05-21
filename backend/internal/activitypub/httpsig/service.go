package httpsig

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// signatureLabel is the structured field member name used for local signatures.
	signatureLabel = "ap"
	// headerSignature carries the RFC 9421 signature bytes.
	headerSignature = "Signature"
	// headerSignatureIn carries the RFC 9421 covered component metadata.
	headerSignatureIn = "Signature-Input"
	// headerContentDigest carries a signed digest for request bodies.
	headerContentDigest = "Content-Digest"
	// headerDate carries the signed request timestamp.
	headerDate = "Date"

	// componentMethod is the RFC 9421 pseudo-component for the HTTP method.
	componentMethod = "@method"
	// componentAuthority is the RFC 9421 pseudo-component for request authority.
	componentAuthority = "@authority"
	// componentPath is the RFC 9421 pseudo-component for request path.
	componentPath = "@path"
	// componentQuery is the RFC 9421 pseudo-component for query string.
	componentQuery = "@query"
	// componentSignatureParams is the synthetic component containing signature parameters.
	componentSignatureParams = "@signature-params"
	// componentContentDigest is the covered Content-Digest header component.
	componentContentDigest = "content-digest"
	// componentDate is the covered Date header component.
	componentDate = "date"
)

var (
	// ErrMissingSignature reports that signature headers are absent or unmatched.
	ErrMissingSignature = errors.New("missing http signature")
	// ErrInvalidSignatureInput reports malformed Signature-Input metadata.
	ErrInvalidSignatureInput = errors.New("invalid signature input")
	// ErrUnsupportedAlgorithm reports a signature algorithm this server will not use.
	ErrUnsupportedAlgorithm = errors.New("unsupported http signature algorithm")
	// ErrInvalidSignature reports a cryptographic verification failure.
	ErrInvalidSignature = errors.New("invalid http signature")
	// ErrInvalidDigest reports a request body digest mismatch.
	ErrInvalidDigest = errors.New("invalid content digest")
	// ErrInvalidDate reports an invalid signed Date header.
	ErrInvalidDate = errors.New("invalid http date")
	// ErrMissingCoveredContent reports a missing required covered request component.
	ErrMissingCoveredContent = errors.New("missing signed content")
	// ErrExpiredSignature reports a signature outside the allowed clock window.
	ErrExpiredSignature = errors.New("expired http signature")
	// ErrMissingCoveredMetadata reports missing signature metadata such as keyid or created.
	ErrMissingCoveredMetadata = errors.New("missing signed metadata")
)

// Service signs outbound requests and verifies inbound HTTP Message Signatures.
type Service struct {
	repo               Repository
	clock              func() time.Time
	maxAge             time.Duration
	missingKeyResolver func(context.Context, string) error
	keyRefreshResolver func(context.Context, string, string) error
}

// Option configures an HTTP signature service.
type Option func(*Service)

// VerifiedRequest describes the actor authenticated by an HTTP signature.
type VerifiedRequest struct {
	ActorID    string
	ActorAPID  string
	KeyID      string
	Algorithm  string
	CreatedAt  time.Time
	Components []string
}

// NewService creates an HTTP signature service.
func NewService(repo Repository, opts ...Option) *Service {
	service := &Service{
		repo:   repo,
		clock:  func() time.Time { return time.Now().UTC() },
		maxAge: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// WithClock overrides time source for signature creation and verification.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithMaxAge sets the accepted signature clock skew window.
func WithMaxAge(maxAge time.Duration) Option {
	return func(s *Service) {
		s.maxAge = maxAge
	}
}

// WithMissingKeyResolver configures a callback to fetch unknown remote keys.
func WithMissingKeyResolver(resolver func(context.Context, string) error) Option {
	return func(s *Service) {
		s.missingKeyResolver = resolver
	}
}

// WithKeyRefreshResolver configures a callback to refresh stale remote keys.
func WithKeyRefreshResolver(resolver func(context.Context, string, string) error) Option {
	return func(s *Service) {
		s.keyRefreshResolver = resolver
	}
}

// SignRequest signs an outbound request as the given local actor.
func (s *Service) SignRequest(ctx context.Context, actorID string, req *http.Request, body []byte) error {
	key, err := s.repo.ActivePrivateKey(ctx, actorID)
	if err != nil {
		return err
	}
	algorithm, err := key.SignatureAlgorithm()
	if err != nil {
		return err
	}
	privateKey, err := ParseRSAPrivateKeyPEM(key.PrivateKeyPEM)
	if err != nil {
		return err
	}

	if req.Header.Get(headerDate) == "" {
		req.Header.Set(headerDate, s.clock().UTC().Format(http.TimeFormat))
	}

	components := []string{componentMethod, componentAuthority, componentPath}
	if req.URL != nil && req.URL.RawQuery != "" {
		components = append(components, componentQuery)
	}
	components = append(components, componentDate)
	if body != nil {
		req.Header.Set(headerContentDigest, contentDigest(body))
		components = append(components, componentContentDigest)
	}

	created := s.clock().UTC().Unix()
	signatureParams := signatureParamsValue(components, created, key.KeyID, algorithm)
	base, err := signatureBase(req, components, signatureParams)
	if err != nil {
		return err
	}
	signature, err := signRSAV15SHA256(privateKey, []byte(base))
	if err != nil {
		return err
	}

	req.Header.Set(headerSignatureIn, signatureLabel+"="+signatureParams)
	req.Header.Set(headerSignature, signatureLabel+"=:"+base64.StdEncoding.EncodeToString(signature)+":")
	return nil
}

// VerifyRequest verifies an inbound signed request and returns the authenticated actor.
func (s *Service) VerifyRequest(ctx context.Context, req *http.Request, body []byte) (*VerifiedRequest, error) {
	inputs, err := parseSignatureInputs(req.Header.Get(headerSignatureIn))
	if err != nil {
		return nil, err
	}
	signatures, err := parseSignatures(req.Header.Get(headerSignature))
	if err != nil {
		return nil, err
	}

	input, signature, ok := matchingSignature(inputs, signatures)
	if !ok {
		return nil, ErrMissingSignature
	}
	if input.Algorithm != AlgorithmRSAV15SHA256 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, input.Algorithm)
	}
	if input.KeyID == "" {
		return nil, fmt.Errorf("%w: keyid", ErrMissingCoveredMetadata)
	}
	if input.Created == 0 {
		return nil, fmt.Errorf("%w: created", ErrMissingCoveredMetadata)
	}

	createdAt := time.Unix(input.Created, 0).UTC()
	if s.maxAge > 0 {
		now := s.clock().UTC()
		if now.Sub(createdAt) > s.maxAge || createdAt.Sub(now) > s.maxAge {
			return nil, ErrExpiredSignature
		}
	}

	if err := requireComponents(input.Components, []string{componentMethod, componentAuthority, componentPath, componentDate}); err != nil {
		return nil, err
	}
	if len(body) > 0 && !containsComponent(input.Components, componentContentDigest) {
		return nil, fmt.Errorf("%w: %s", ErrMissingCoveredContent, componentContentDigest)
	}
	if err := verifyDate(req.Header.Get(headerDate), s.clock().UTC(), s.maxAge); err != nil {
		return nil, err
	}
	if containsComponent(input.Components, componentContentDigest) {
		if err := verifyContentDigest(req.Header.Get(headerContentDigest), body); err != nil {
			return nil, err
		}
	}

	key, err := s.repo.PublicKeyByKeyID(ctx, input.KeyID)
	if errors.Is(err, sql.ErrNoRows) && s.missingKeyResolver != nil {
		if resolveErr := s.missingKeyResolver(ctx, input.KeyID); resolveErr != nil {
			return nil, resolveErr
		}
		key, err = s.repo.PublicKeyByKeyID(ctx, input.KeyID)
	}
	if err != nil {
		return nil, err
	}
	base, err := signatureBase(req, input.Components, input.RawValue)
	if err != nil {
		return nil, err
	}
	algorithm, err := s.verifyWithKey(key, input.Algorithm, []byte(base), signature)
	if errors.Is(err, ErrInvalidSignature) && s.keyRefreshResolver != nil {
		originalActorID := key.ActorID
		originalActorAPID := key.ActorAPID
		if refreshErr := s.keyRefreshResolver(ctx, input.KeyID, originalActorAPID); refreshErr != nil {
			return nil, err
		}
		key, err = s.repo.PublicKeyByKeyID(ctx, input.KeyID)
		if err != nil {
			return nil, err
		}
		if key.ActorID != originalActorID || key.ActorAPID != originalActorAPID {
			return nil, ErrInvalidSignature
		}
		algorithm, err = s.verifyWithKey(key, input.Algorithm, []byte(base), signature)
	}
	if err != nil {
		return nil, err
	}

	return &VerifiedRequest{
		ActorID:    key.ActorID,
		ActorAPID:  key.ActorAPID,
		KeyID:      key.KeyID,
		Algorithm:  algorithm,
		CreatedAt:  createdAt,
		Components: append([]string(nil), input.Components...),
	}, nil
}

// verifyWithKey checks the expected algorithm and verifies the request signature.
func (s *Service) verifyWithKey(key *ActorKey, expectedAlgorithm string, base []byte, signature []byte) (string, error) {
	algorithm, err := key.SignatureAlgorithm()
	if err != nil {
		return "", err
	}
	if algorithm != expectedAlgorithm {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, key.Algorithm)
	}
	publicKey, err := ParseRSAPublicKeyPEM(key.PublicKeyPEM)
	if err != nil {
		return "", err
	}
	if err := verifyRSAV15SHA256(publicKey, base, signature); err != nil {
		return "", err
	}
	return algorithm, nil
}

// signRSAV15SHA256 signs a signature base with RSA PKCS#1 v1.5 and SHA-256.
func signRSAV15SHA256(key *rsa.PrivateKey, base []byte) ([]byte, error) {
	sum := sha256.Sum256(base)
	return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
}

// verifyRSAV15SHA256 verifies an RSA PKCS#1 v1.5 SHA-256 signature.
func verifyRSAV15SHA256(key *rsa.PublicKey, base []byte, signature []byte) error {
	sum := sha256.Sum256(base)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
		return ErrInvalidSignature
	}
	return nil
}

// signatureBase builds the RFC 9421 signature base from covered components.
func signatureBase(req *http.Request, components []string, signatureParams string) (string, error) {
	lines := make([]string, 0, len(components)+1)
	for _, component := range components {
		value, err := componentValue(req, component)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%q: %s", component, value))
	}
	lines = append(lines, fmt.Sprintf("%q: %s", componentSignatureParams, signatureParams))
	return strings.Join(lines, "\n"), nil
}

// componentValue returns the canonical value for a signed request component.
func componentValue(req *http.Request, component string) (string, error) {
	switch component {
	case componentMethod:
		return req.Method, nil
	case componentAuthority:
		scheme := ""
		if req.URL != nil {
			scheme = req.URL.Scheme
		}
		if req.Host != "" {
			return normalizeAuthority(req.Host, scheme), nil
		}
		if req.URL != nil && req.URL.Host != "" {
			return normalizeAuthority(req.URL.Host, scheme), nil
		}
		return "", fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
	case componentPath:
		path := "/"
		if req.URL != nil {
			path = req.URL.EscapedPath()
			if path == "" {
				path = "/"
			}
		}
		return path, nil
	case componentQuery:
		if req.URL == nil || req.URL.RawQuery == "" {
			return "?", nil
		}
		return "?" + req.URL.RawQuery, nil
	case componentDate:
		value := req.Header.Get(headerDate)
		if value == "" {
			return "", fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
		}
		return value, nil
	case componentContentDigest:
		value := req.Header.Get(headerContentDigest)
		if value == "" {
			return "", fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
		}
		return value, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
	}
}

// normalizeAuthority normalizes signed Host authority values for default ports.
func normalizeAuthority(authority, scheme string) string {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return ""
	}

	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		return strings.ToLower(authority)
	}

	host = strings.ToLower(host)
	scheme = strings.ToLower(scheme)
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		return host
	}
	return net.JoinHostPort(host, port)
}

// signatureParamsValue renders the RFC 9421 Signature-Input parameter value.
func signatureParamsValue(components []string, created int64, keyID, algorithm string) string {
	quoted := make([]string, 0, len(components))
	for _, component := range components {
		quoted = append(quoted, sfString(component))
	}
	return "(" + strings.Join(quoted, " ") + ");created=" + fmt.Sprint(created) + ";keyid=" + sfString(keyID) + ";alg=" + sfString(algorithm)
}

// contentDigest returns the SHA-256 Content-Digest header value for a body.
func contentDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha-256=:" + base64.StdEncoding.EncodeToString(sum[:]) + ":"
}

// verifyContentDigest compares a received Content-Digest header with the body.
func verifyContentDigest(header string, body []byte) error {
	if header == "" {
		return fmt.Errorf("%w: missing content-digest", ErrInvalidDigest)
	}
	if header != contentDigest(body) {
		return ErrInvalidDigest
	}
	return nil
}

// verifyDate parses and bounds the signed Date header.
func verifyDate(header string, now time.Time, maxAge time.Duration) error {
	signedAt, err := http.ParseTime(header)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDate, err)
	}
	if maxAge > 0 {
		signedAt = signedAt.UTC()
		if now.Sub(signedAt) > maxAge || signedAt.Sub(now) > maxAge {
			return ErrExpiredSignature
		}
	}
	return nil
}

// requireComponents ensures a signature covers all required HTTP components.
func requireComponents(components []string, required []string) error {
	for _, component := range required {
		if !containsComponent(components, component) {
			return fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
		}
	}
	return nil
}

// containsComponent reports whether a component list contains a specific value.
func containsComponent(components []string, target string) bool {
	for _, component := range components {
		if component == target {
			return true
		}
	}
	return false
}

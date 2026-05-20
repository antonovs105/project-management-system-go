package httpsig

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	signatureLabel      = "ap"
	headerSignature     = "Signature"
	headerSignatureIn   = "Signature-Input"
	headerContentDigest = "Content-Digest"
	headerDate          = "Date"

	componentMethod          = "@method"
	componentAuthority       = "@authority"
	componentPath            = "@path"
	componentQuery           = "@query"
	componentSignatureParams = "@signature-params"
	componentContentDigest   = "content-digest"
	componentDate            = "date"
)

var (
	ErrMissingSignature       = errors.New("missing http signature")
	ErrInvalidSignatureInput  = errors.New("invalid signature input")
	ErrUnsupportedAlgorithm   = errors.New("unsupported http signature algorithm")
	ErrInvalidSignature       = errors.New("invalid http signature")
	ErrInvalidDigest          = errors.New("invalid content digest")
	ErrMissingCoveredContent  = errors.New("missing signed content")
	ErrExpiredSignature       = errors.New("expired http signature")
	ErrMissingCoveredMetadata = errors.New("missing signed metadata")
)

type Service struct {
	repo   Repository
	clock  func() time.Time
	maxAge time.Duration
}

type Option func(*Service)

type VerifiedRequest struct {
	ActorID    string
	KeyID      string
	Algorithm  string
	CreatedAt  time.Time
	Components []string
}

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

func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func WithMaxAge(maxAge time.Duration) Option {
	return func(s *Service) {
		s.maxAge = maxAge
	}
}

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
	if containsComponent(input.Components, componentContentDigest) {
		if err := verifyContentDigest(req.Header.Get(headerContentDigest), body); err != nil {
			return nil, err
		}
	}

	key, err := s.repo.PublicKeyByKeyID(ctx, input.KeyID)
	if err != nil {
		return nil, err
	}
	algorithm, err := key.SignatureAlgorithm()
	if err != nil {
		return nil, err
	}
	if algorithm != input.Algorithm {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, key.Algorithm)
	}
	publicKey, err := ParseRSAPublicKeyPEM(key.PublicKeyPEM)
	if err != nil {
		return nil, err
	}

	base, err := signatureBase(req, input.Components, input.RawValue)
	if err != nil {
		return nil, err
	}
	if err := verifyRSAV15SHA256(publicKey, []byte(base), signature); err != nil {
		return nil, err
	}

	return &VerifiedRequest{
		ActorID:    key.ActorID,
		KeyID:      key.KeyID,
		Algorithm:  algorithm,
		CreatedAt:  createdAt,
		Components: append([]string(nil), input.Components...),
	}, nil
}

func signRSAV15SHA256(key *rsa.PrivateKey, base []byte) ([]byte, error) {
	sum := sha256.Sum256(base)
	return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
}

func verifyRSAV15SHA256(key *rsa.PublicKey, base []byte, signature []byte) error {
	sum := sha256.Sum256(base)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
		return ErrInvalidSignature
	}
	return nil
}

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

func signatureParamsValue(components []string, created int64, keyID, algorithm string) string {
	quoted := make([]string, 0, len(components))
	for _, component := range components {
		quoted = append(quoted, sfString(component))
	}
	return "(" + strings.Join(quoted, " ") + ");created=" + fmt.Sprint(created) + ";keyid=" + sfString(keyID) + ";alg=" + sfString(algorithm)
}

func contentDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha-256=:" + base64.StdEncoding.EncodeToString(sum[:]) + ":"
}

func verifyContentDigest(header string, body []byte) error {
	if header == "" {
		return fmt.Errorf("%w: missing content-digest", ErrInvalidDigest)
	}
	if header != contentDigest(body) {
		return ErrInvalidDigest
	}
	return nil
}

func requireComponents(components []string, required []string) error {
	for _, component := range required {
		if !containsComponent(components, component) {
			return fmt.Errorf("%w: %s", ErrMissingCoveredContent, component)
		}
	}
	return nil
}

func containsComponent(components []string, target string) bool {
	for _, component := range components {
		if component == target {
			return true
		}
	}
	return false
}

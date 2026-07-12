package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/antonovs105/project-management-system-go/internal/secrets"
)

var (
	// ErrInvalidToken reports a missing, expired, or already consumed account challenge.
	ErrInvalidToken = errors.New("invalid or expired account token")
	// ErrInvalidPassword reports a password that does not meet the local policy.
	ErrInvalidPassword = errors.New("password must contain 8 to 72 bytes")
	// ErrMFARequired reports that a valid authenticator or recovery code is required.
	ErrMFARequired = errors.New("multi-factor authentication code is required")
	// ErrMFAInvalid reports an invalid authenticator or already-used recovery code.
	ErrMFAInvalid = errors.New("invalid multi-factor authentication code")
	// ErrMFANotConfigured reports an enrollment lifecycle request in the wrong state.
	ErrMFANotConfigured = errors.New("multi-factor authentication is not configured")
)

const (
	// verificationTTL bounds email ownership links.
	verificationTTL = 24 * time.Hour
	// passwordResetTTL bounds password recovery links.
	passwordResetTTL = 30 * time.Minute
	// sessionLifetime matches the browser authentication cookie lifetime.
	sessionLifetime = 12 * time.Hour
)

// Service implements account recovery and session lifecycle behavior.
type Service struct {
	repository  *Repository
	publicURL   string
	productName string
	mailer      Mailer
	development bool
	secretCodec secrets.PrivateKeyCodec
}

// Option customizes account security dependencies.
type Option func(*Service)

// WithSecretCodec encrypts MFA secrets before persistence.
func WithSecretCodec(codec secrets.PrivateKeyCodec) Option {
	return func(service *Service) {
		if codec != nil {
			service.secretCodec = codec
		}
	}
}

// NewService returns an account security service.
func NewService(repository *Repository, publicURL, productName string, mailer Mailer, development bool, options ...Option) *Service {
	service := &Service{
		repository:  repository,
		publicURL:   strings.TrimRight(strings.TrimSpace(publicURL), "/"),
		productName: strings.TrimSpace(productName),
		mailer:      mailer,
		development: development,
		secretCodec: secrets.NoopPrivateKeyCodec{},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// SendRegistrationVerification queues an email-ownership challenge for a new local user.
func (s *Service) SendRegistrationVerification(ctx context.Context, userID, email string) error {
	return s.queueChallenge(ctx, userID, email, TokenPurposeVerifyEmail, verificationTTL)
}

// RequestVerification queues a replacement challenge for an authenticated unverified user.
func (s *Service) RequestVerification(ctx context.Context, userID string) error {
	email, verified, err := s.repository.UserEmail(ctx, userID)
	if err != nil || verified {
		return err
	}
	return s.queueChallenge(ctx, userID, email, TokenPurposeVerifyEmail, verificationTTL)
}

// VerifyEmail consumes a single-use challenge.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidToken
	}
	_, err := s.repository.ConsumeEmailVerification(ctx, hashToken(token), time.Now().UTC())
	if IsNotFound(err) {
		return ErrInvalidToken
	}
	return err
}

// RequestPasswordReset is enumeration-resistant and queues mail only for known local users.
func (s *Service) RequestPasswordReset(ctx context.Context, email string, client ClientInfo) error {
	email = normalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil
	}
	userID, storedEmail, _, err := s.repository.FindUserByEmail(ctx, email)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.repository.RecordAuthEvent(ctx, userID, "password.reset_requested", client, nil); err != nil {
		return err
	}
	return s.queueChallenge(ctx, userID, storedEmail, TokenPurposeResetPassword, passwordResetTTL)
}

// ResetPassword consumes a challenge, validates the replacement, and revokes all sessions.
func (s *Service) ResetPassword(ctx context.Context, token, password string, client ClientInfo) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidToken
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.repository.ConsumePasswordReset(ctx, hashToken(token), string(hash), time.Now().UTC(), client)
	if IsNotFound(err) {
		return ErrInvalidToken
	}
	return err
}

// CreateSession persists a server-visible session and returns its identifier.
func (s *Service) CreateSession(ctx context.Context, userID string, client ClientInfo) (string, error) {
	sessionID := uuid.NewString()
	if err := s.repository.CreateSession(ctx, sessionID, userID, client, time.Now().UTC().Add(sessionLifetime)); err != nil {
		return "", err
	}
	return sessionID, nil
}

// ValidateSession confirms that a session claim is still active.
func (s *Service) ValidateSession(ctx context.Context, userID, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrInvalidToken
	}
	return s.repository.ValidateSession(ctx, sessionID, userID, time.Now().UTC())
}

// ListSessions returns session inventory and marks the current browser session.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID string) ([]Session, error) {
	values, err := s.repository.ListSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	for index := range values {
		values[index].Current = values[index].ID == currentSessionID
	}
	return values, nil
}

// RevokeSession invalidates one owned session.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string, client ClientInfo) error {
	err := s.repository.RevokeSession(ctx, userID, sessionID, client)
	if IsNotFound(err) {
		return ErrInvalidToken
	}
	return err
}

// RecordAuthEvent durably appends a security event.
func (s *Service) RecordAuthEvent(ctx context.Context, userID, eventType string, client ClientInfo) error {
	return s.repository.RecordAuthEvent(ctx, userID, eventType, client, nil)
}

// ListAuthEvents returns recent security events owned by a user.
func (s *Service) ListAuthEvents(ctx context.Context, userID string) ([]SecurityEvent, error) {
	return s.repository.ListAuthEvents(ctx, userID)
}

// MFAStatus reports whether the current user has an active second factor.
func (s *Service) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	_, enabled, err := s.repository.MFACredential(ctx, userID)
	if IsNotFound(err) {
		return MFAStatus{}, nil
	}
	return MFAStatus{Enabled: enabled}, err
}

// BeginMFA replaces any unconfirmed setup and returns a new authenticator secret.
func (s *Service) BeginMFA(ctx context.Context, userID string) (MFASetup, error) {
	email, _, err := s.repository.UserEmail(ctx, userID)
	if err != nil {
		return MFASetup{}, err
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		return MFASetup{}, err
	}
	ciphertext, err := s.secretCodec.EncryptPrivateKey(secret)
	if err != nil {
		return MFASetup{}, err
	}
	if err := s.repository.SaveMFASetup(ctx, userID, ciphertext); err != nil {
		return MFASetup{}, err
	}
	return MFASetup{Secret: secret, URI: totpURI(secret, s.productName, email)}, nil
}

// ConfirmMFA proves possession and returns recovery codes exactly once.
func (s *Service) ConfirmMFA(ctx context.Context, userID, code string, client ClientInfo) (MFARecoveryCodes, error) {
	secret, enabled, err := s.mfaSecret(ctx, userID)
	if err != nil || enabled {
		if err == nil {
			err = ErrMFANotConfigured
		}
		return MFARecoveryCodes{}, err
	}
	if !validateTOTP(secret, code, time.Now().UTC()) {
		return MFARecoveryCodes{}, ErrMFAInvalid
	}
	codes, err := generateRecoveryCodes(10)
	if err != nil {
		return MFARecoveryCodes{}, err
	}
	hashes := make([]string, len(codes))
	for index := range codes {
		hashes[index] = hashToken(normalizeRecoveryCode(codes[index]))
	}
	if err := s.repository.EnableMFA(ctx, userID, hashes, client); err != nil {
		return MFARecoveryCodes{}, err
	}
	return MFARecoveryCodes{RecoveryCodes: codes}, nil
}

// VerifyMFA accepts a current TOTP or consumes a one-time recovery code.
func (s *Service) VerifyMFA(ctx context.Context, userID, code string) error {
	secret, enabled, err := s.mfaSecret(ctx, userID)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrMFARequired
	}
	if validateTOTP(secret, code, time.Now().UTC()) {
		return nil
	}
	if err := s.repository.ConsumeRecoveryCode(ctx, userID, hashToken(normalizeRecoveryCode(code))); err == nil {
		return nil
	}
	return ErrMFAInvalid
}

// DisableMFA verifies the existing factor before deleting it.
func (s *Service) DisableMFA(ctx context.Context, userID, code string, client ClientInfo) error {
	if err := s.VerifyMFA(ctx, userID, code); err != nil {
		return err
	}
	err := s.repository.DisableMFA(ctx, userID, client)
	if IsNotFound(err) {
		return ErrMFANotConfigured
	}
	return err
}

// HasMFA reports whether an account is safe to promote to a privileged role.
func (s *Service) HasMFA(ctx context.Context, userID string) (bool, error) {
	status, err := s.MFAStatus(ctx, userID)
	return status.Enabled, err
}

// mfaSecret decrypts the stored authenticator secret.
func (s *Service) mfaSecret(ctx context.Context, userID string) (string, bool, error) {
	ciphertext, enabled, err := s.repository.MFACredential(ctx, userID)
	if err != nil {
		return "", false, err
	}
	secret, err := s.secretCodec.DecryptPrivateKey(ciphertext)
	return secret, enabled, err
}

// normalizeRecoveryCode removes visual separators before hashing.
func normalizeRecoveryCode(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

// StartEmailDispatcher delivers transactional outbox messages until stopped.
func (s *Service) StartEmailDispatcher(ctx context.Context, interval time.Duration) func() {
	if s.mailer == nil {
		log.Printf("transactional email dispatcher disabled: SMTP is not configured")
		return func() {}
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	workerContext, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := s.deliverOne(workerContext); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("transactional email delivery error: %v", err)
			}
			select {
			case <-workerContext.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

// deliverOne leases and delivers at most one outbox message.
func (s *Service) deliverOne(ctx context.Context) error {
	message, err := s.repository.ClaimEmail(ctx, time.Now().UTC())
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.mailer.Send(ctx, *message); err != nil {
		if recordErr := s.repository.FailEmail(ctx, message.ID, err.Error(), message.Attempts); recordErr != nil {
			return fmt.Errorf("send failed: %v; retry persistence failed: %w", err, recordErr)
		}
		return err
	}
	return s.repository.CompleteEmail(ctx, message.ID)
}

// queueChallenge generates a random single-use token and atomically stores its email.
func (s *Service) queueChallenge(ctx context.Context, userID, email, purpose string, ttl time.Duration) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	path := "/auth/verify-email"
	subject := "Verify your email"
	if purpose == TokenPurposeResetPassword {
		path = "/auth/reset-password"
		subject = "Reset your password"
	}
	link := s.publicURL + path + "?token=" + url.QueryEscape(token)
	body := fmt.Sprintf("Use this single-use link to continue with %s:\n\n%s\n\nIf you did not request this, you can ignore this message.", s.productName, link)
	if s.development {
		log.Printf("development account link recipient=%s purpose=%s url=%s", email, purpose, link)
	}
	return s.repository.ReplaceToken(ctx, userID, purpose, hashToken(token), email, subject, body, time.Now().UTC().Add(ttl))
}

// randomToken creates a URL-safe cryptographically random value.
func randomToken(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// hashToken prevents database disclosure from revealing active account links.
func hashToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// normalizeEmail applies the same comparison policy as local login.
func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// validatePassword enforces bcrypt's input bound and the product minimum.
func validatePassword(value string) error {
	if utf8.RuneCountInString(value) < 8 || len(value) > 72 {
		return ErrInvalidPassword
	}
	return nil
}

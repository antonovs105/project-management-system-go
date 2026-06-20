package user

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidUserInput reports a malformed user-management request.
var ErrInvalidUserInput = errors.New("invalid user input")

// ErrAdminAlreadyExists reports that bootstrap cannot create another first owner.
var ErrAdminAlreadyExists = errors.New("admin user already exists")

// ErrAdminRequired reports that the current user lacks admin privileges.
var ErrAdminRequired = errors.New("admin privileges required")

// ErrOwnerRequired reports that the current user lacks owner privileges.
var ErrOwnerRequired = errors.New("owner privileges required")

// ErrUserNotFound reports that the target user does not exist locally.
var ErrUserNotFound = errors.New("user not found")

// ErrCannotDemoteLastAdmin protects the instance from losing its final owner.
var ErrCannotDemoteLastAdmin = errors.New("cannot demote the last instance owner")

// ErrInvalidCredentials reports failed authentication without revealing which credential failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrRegistrationDisabled reports that this instance is closed to new accounts.
var ErrRegistrationDisabled = errors.New("registration is disabled")

// ErrOAuthProviderUnavailable reports that a requested OAuth provider is disabled.
var ErrOAuthProviderUnavailable = errors.New("oauth provider unavailable")

// ErrOAuthInvalidState reports an invalid OAuth state token.
var ErrOAuthInvalidState = errors.New("invalid oauth state")

// ErrOAuthProviderFailed reports an upstream OAuth exchange or profile failure.
var ErrOAuthProviderFailed = errors.New("oauth provider request failed")

// ErrOAuthEmailNotVerified reports that the provider did not verify the email address.
var ErrOAuthEmailNotVerified = errors.New("oauth email is not verified")

// ErrOAuthEmailAlreadyRegistered reports that OAuth attempted to claim a local email account.
var ErrOAuthEmailAlreadyRegistered = errors.New("email is already registered; sign in and link this provider")

// ErrOAuthInvalidCode reports a missing, expired, or already-used OAuth login exchange code.
var ErrOAuthInvalidCode = errors.New("invalid oauth login code")

const (
	// InstanceRoleOwner allows full instance administration, including owner assignment.
	InstanceRoleOwner = "owner"
	// InstanceRoleAdmin allows global administrative operations.
	InstanceRoleAdmin = "admin"
	// InstanceRoleUser allows normal project-management operations.
	InstanceRoleUser = "user"

	// defaultAdminListLimit is the fallback admin user list size.
	defaultAdminListLimit = 100
	// maxAdminListLimit is the largest accepted admin user list size.
	maxAdminListLimit = 500
	// minPasswordLength is the shortest accepted local password.
	minPasswordLength = 8
	// maxPasswordBytes is bcrypt's maximum accepted password input length.
	maxPasswordBytes = 72

	// OAuthProviderGoogle is the provider key for Google OpenID Connect login.
	OAuthProviderGoogle = "google"
	// OAuthProviderGitHub is the provider key for GitHub OAuth login.
	OAuthProviderGitHub = "github"

	// defaultOAuthStateTTL bounds how long signed OAuth state values are accepted.
	defaultOAuthStateTTL = 10 * time.Minute
	// defaultOAuthCodeTTL bounds how long frontend exchange codes can be consumed.
	defaultOAuthCodeTTL = 5 * time.Minute
)

// Service encapsulates local user registration, login, and account management.
type Service struct {
	repo                Repository
	jwtSecretKey        []byte
	apConfig            activitypub.Config
	oauthConfig         OAuthConfig
	httpClient          *http.Client
	registrationEnabled bool
}

// OAuthProviderConfig contains one provider's OAuth client settings.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OAuthConfig contains optional provider login settings.
type OAuthConfig struct {
	FrontendCallbackURL string
	Google              OAuthProviderConfig
	GitHub              OAuthProviderConfig
	StateTTL            time.Duration
	CodeTTL             time.Duration
}

// Option customizes the user service.
type Option func(*Service)

// WithOAuthConfig enables optional OAuth providers.
func WithOAuthConfig(config OAuthConfig) Option {
	return func(s *Service) {
		s.oauthConfig = config
	}
}

// WithHTTPClient replaces the default HTTP client, primarily for tests.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		if client != nil {
			s.httpClient = client
		}
	}
}

// WithRegistrationEnabled controls whether this instance accepts new accounts.
func WithRegistrationEnabled(enabled bool) Option {
	return func(s *Service) {
		s.registrationEnabled = enabled
	}
}

// NewService creates a user service with repository, JWT secret, and ActivityPub configuration.
func NewService(repo Repository, jwtSecret []byte, apConfig activitypub.Config, options ...Option) *Service {
	service := &Service{
		repo:                repo,
		jwtSecretKey:        jwtSecret,
		apConfig:            apConfig,
		httpClient:          &http.Client{Timeout: 10 * time.Second},
		registrationEnabled: true,
	}
	for _, option := range options {
		option(service)
	}
	if service.oauthConfig.FrontendCallbackURL == "" {
		service.oauthConfig.FrontendCallbackURL = "http://localhost:5173/oauth/callback"
	}
	if service.oauthConfig.StateTTL <= 0 {
		service.oauthConfig.StateTTL = defaultOAuthStateTTL
	}
	if service.oauthConfig.CodeTTL <= 0 {
		service.oauthConfig.CodeTTL = defaultOAuthCodeTTL
	}
	return service
}

// RegisterUser creates a local regular account and its ActivityPub actor graph.
func (s *Service) RegisterUser(ctx context.Context, username, email, password string) (*User, error) {
	if !s.registrationEnabled {
		return nil, ErrRegistrationDisabled
	}
	newUser, err := s.newLocalUser(username, email, password, InstanceRoleUser)
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateUser(ctx, newUser)
	if err != nil {
		return nil, err
	}

	newUser.PasswordHash = ""
	return newUser, nil
}

// EnabledOAuthProviders returns providers with complete client settings.
func (s *Service) EnabledOAuthProviders() []string {
	providers := make([]string, 0, 2)
	if s.oauthProviderConfig(OAuthProviderGoogle).configured() {
		providers = append(providers, OAuthProviderGoogle)
	}
	if s.oauthProviderConfig(OAuthProviderGitHub).configured() {
		providers = append(providers, OAuthProviderGitHub)
	}
	return providers
}

// OAuthStartURL returns the provider authorization URL and signed state.
func (s *Service) OAuthStartURL(provider string) (string, error) {
	provider = normalizeOAuthProvider(provider)
	cfg := s.oauthProviderConfig(provider)
	if !cfg.configured() {
		return "", ErrOAuthProviderUnavailable
	}
	nonce, err := randomToken(18)
	if err != nil {
		return "", err
	}
	state, err := s.signOAuthState(oauthState{
		Provider:  provider,
		ExpiresAt: time.Now().Add(s.oauthConfig.StateTTL).Unix(),
		Nonce:     nonce,
	})
	if err != nil {
		return "", err
	}
	switch provider {
	case OAuthProviderGoogle:
		return oauthURL("https://accounts.google.com/o/oauth2/v2/auth", map[string]string{
			"client_id":     cfg.ClientID,
			"redirect_uri":  cfg.RedirectURL,
			"response_type": "code",
			"scope":         "openid email profile",
			"state":         state,
			"prompt":        "select_account",
		}), nil
	case OAuthProviderGitHub:
		return oauthURL("https://github.com/login/oauth/authorize", map[string]string{
			"client_id":    cfg.ClientID,
			"redirect_uri": cfg.RedirectURL,
			"scope":        "read:user user:email",
			"state":        state,
		}), nil
	default:
		return "", ErrOAuthProviderUnavailable
	}
}

// CompleteOAuthLogin exchanges provider code, creates/loads a local user, and returns a one-time frontend code.
func (s *Service) CompleteOAuthLogin(ctx context.Context, provider, code, signedState string) (string, error) {
	provider = normalizeOAuthProvider(provider)
	if !s.oauthProviderConfig(provider).configured() {
		return "", ErrOAuthProviderUnavailable
	}
	state, err := s.verifyOAuthState(signedState)
	if err != nil {
		return "", err
	}
	if state.Provider != provider {
		return "", ErrOAuthInvalidState
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", ErrOAuthProviderFailed
	}
	profile, err := s.fetchOAuthProfile(ctx, provider, code)
	if err != nil {
		return "", err
	}
	user, err := s.userForOAuthProfile(ctx, profile)
	if err != nil {
		return "", err
	}
	rawCode, err := randomToken(32)
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateOAuthLoginCode(ctx, user.ID, hashOAuthCode(rawCode), time.Now().Add(s.oauthConfig.CodeTTL)); err != nil {
		return "", err
	}
	return rawCode, nil
}

// ExchangeOAuthLoginCode consumes a one-time frontend code and returns a normal JWT.
func (s *Service) ExchangeOAuthLoginCode(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", ErrOAuthInvalidCode
	}
	user, err := s.repo.ConsumeOAuthLoginCode(ctx, hashOAuthCode(code), time.Now())
	if err != nil {
		return "", err
	}
	return s.issueToken(user)
}

// OAuthFrontendRedirectURL builds the SPA callback URL with a one-time code or safe error code.
func (s *Service) OAuthFrontendRedirectURL(code, errorCode string) string {
	callback := s.oauthConfig.FrontendCallbackURL
	parsed, err := url.Parse(callback)
	if err != nil {
		parsed, _ = url.Parse("http://localhost:5173/oauth/callback")
	}
	query := parsed.Query()
	if strings.TrimSpace(code) != "" {
		query.Set("code", code)
	}
	if strings.TrimSpace(errorCode) != "" {
		query.Set("error", errorCode)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// BootstrapAdmin creates the first local owner account. The repository enforces
// the one-owner bootstrap guard transactionally.
func (s *Service) BootstrapAdmin(ctx context.Context, username, email, password string) (*User, error) {
	newUser, err := s.newLocalUser(username, email, password, InstanceRoleOwner)
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateAdminIfNoAdmin(ctx, newUser)
	if err != nil {
		return nil, err
	}

	newUser.PasswordHash = ""
	return newUser, nil
}

// ListUsers returns a filtered admin-only list of local users.
func (s *Service) ListUsers(ctx context.Context, adminUserID string, options ListUsersOptions) ([]User, error) {
	if err := s.requireAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}
	options.InstanceRole = strings.ToLower(strings.TrimSpace(options.InstanceRole))
	if options.InstanceRole != "" && !IsValidInstanceRole(options.InstanceRole) {
		return nil, invalidUserInput("invalid instance role")
	}
	options.Query = strings.TrimSpace(options.Query)
	options.Limit = normalizeListLimit(options.Limit)
	options.Offset = normalizeOffset(options.Offset)
	return s.repo.ListUsers(ctx, options)
}

// UpdateInstanceRole changes a local user's instance role and records an audit event.
func (s *Service) UpdateInstanceRole(ctx context.Context, adminUserID, targetUserID, role string) (*User, error) {
	if err := s.requireAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}
	targetUserID = strings.TrimSpace(targetUserID)
	if _, err := uuid.Parse(targetUserID); err != nil {
		return nil, invalidUserInput("valid user id is required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if !IsValidInstanceRole(role) {
		return nil, invalidUserInput("invalid instance role")
	}
	return s.repo.UpdateInstanceRole(ctx, adminUserID, targetUserID, role)
}

// InstanceRole returns the system-wide role for a local user.
func (s *Service) InstanceRole(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrUserNotFound
	}
	role, err := s.repo.InstanceRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrUserNotFound
		}
		return "", err
	}
	return role, nil
}

// ChangePassword replaces a user's password and invalidates their existing JWTs.
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	userID = strings.TrimSpace(userID)
	if _, err := uuid.Parse(userID); err != nil {
		return ErrInvalidCredentials
	}
	if strings.TrimSpace(currentPassword) == "" {
		return invalidUserInput("current password is required")
	}
	if err := validatePassword(currentPassword); err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	existingUser, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrUserNotFound) {
			log.Printf("change password credential lookup failed: %v", err)
		}
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(existingUser.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(existingUser.PasswordHash), []byte(newPassword)); err == nil {
		return invalidUserInput("new password must be different")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePasswordHash(ctx, userID, string(hashedPassword)); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidCredentials
		}
		return err
	}
	return nil
}

// ValidateTokenVersion rejects JWTs whose token_version claim is no longer current.
func (s *Service) ValidateTokenVersion(ctx context.Context, userID string, tokenVersion int) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || tokenVersion <= 0 {
		return ErrInvalidCredentials
	}
	currentVersion, err := s.repo.TokenVersion(ctx, userID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrUserNotFound) {
			log.Printf("jwt token version validation failed: %v", err)
		}
		return ErrInvalidCredentials
	}
	if currentVersion != tokenVersion {
		return ErrInvalidCredentials
	}
	return nil
}

// newLocalUser validates account input and prepares actor/key data before persistence.
func (s *Service) newLocalUser(username, email, password, instanceRole string) (*User, error) {
	username = strings.TrimSpace(username)
	email = normalizeEmail(email)
	if err := validateRegistrationInput(username, email, password); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	userID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	if err != nil {
		return nil, err
	}

	newUser := &User{
		ID:            userID,
		APID:          activitypub.UserAPID(s.apConfig, username),
		Username:      username,
		Email:         email,
		PasswordHash:  string(hashedPassword),
		InstanceRole:  instanceRole,
		Handle:        activitypub.Handle(username, s.apConfig),
		Name:          username,
		Summary:       "",
		PublicKeyPEM:  publicKey,
		PrivateKeyPEM: privateKey,
	}

	return newUser, nil
}

// newOAuthUser prepares a local user and ActivityPub actor graph for a provider profile.
func (s *Service) newOAuthUser(profile OAuthProfile) (*User, error) {
	username := oauthUsername(profile.Provider, profile.Subject)
	email := normalizeEmail(profile.Email)
	if err := validateOAuthUserInput(username, email); err != nil {
		return nil, err
	}
	randomPassword, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	randomPasswordHash, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	userID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(profile.DisplayName)
	if name == "" {
		name = username
	}
	return &User{
		ID:            userID,
		APID:          activitypub.UserAPID(s.apConfig, username),
		Username:      username,
		Email:         email,
		PasswordHash:  string(randomPasswordHash),
		InstanceRole:  InstanceRoleUser,
		Handle:        activitypub.Handle(username, s.apConfig),
		Name:          name,
		Summary:       "",
		PublicKeyPEM:  publicKey,
		PrivateKeyPEM: privateKey,
	}, nil
}

// validateRegistrationInput validates public registration and bootstrap account fields.
func validateRegistrationInput(username, email, password string) error {
	if username == "" {
		return invalidUserInput("username is required")
	}
	if strings.ContainsAny(username, "/@ \t\r\n") {
		return invalidUserInput("username may not contain spaces, slashes, or @")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return invalidUserInput("valid email is required")
	}
	return validatePassword(password)
}

// validateOAuthUserInput checks provider-derived fields before local account creation.
func validateOAuthUserInput(username, email string) error {
	if username == "" {
		return invalidUserInput("username is required")
	}
	if strings.ContainsAny(username, "/@ \t\r\n") {
		return invalidUserInput("username may not contain spaces, slashes, or @")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return invalidUserInput("valid email is required")
	}
	return nil
}

// normalizeEmail trims and lowercases an email address for uniqueness and login.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validatePassword enforces the local password length policy.
func validatePassword(password string) error {
	passwordLength := utf8.RuneCountInString(password)
	if passwordLength < minPasswordLength {
		return invalidUserInput(fmt.Sprintf("password must be at least %d characters", minPasswordLength))
	}
	if len(password) > maxPasswordBytes {
		return invalidUserInput(fmt.Sprintf("password must be at most %d bytes", maxPasswordBytes))
	}
	return nil
}

// IsValidInstanceRole reports whether role is a supported system-wide user role.
func IsValidInstanceRole(role string) bool {
	return role == InstanceRoleOwner || role == InstanceRoleAdmin || role == InstanceRoleUser
}

// HasAdminPrivileges reports whether role can use instance administration endpoints.
func HasAdminPrivileges(role string) bool {
	return role == InstanceRoleOwner || role == InstanceRoleAdmin
}

// normalizeListLimit clamps admin user list limits to a bounded default range.
func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return defaultAdminListLimit
	}
	if limit > maxAdminListLimit {
		return maxAdminListLimit
	}
	return limit
}

// normalizeOffset returns zero for negative pagination offsets.
func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// requireAdmin verifies that userID belongs to an instance owner or admin account.
func (s *Service) requireAdmin(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAdminRequired
	}
	role, err := s.repo.InstanceRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAdminRequired
		}
		return err
	}
	if !HasAdminPrivileges(role) {
		return ErrAdminRequired
	}
	return nil
}

// invalidUserInput wraps a validation message with the shared user input sentinel.
func invalidUserInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidUserInput, message)
}

// Login verifies credentials and returns a JWT bound to the user's token version.
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	email = normalizeEmail(email)

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrUserNotFound) {
			log.Printf("login credential lookup failed: %v", err)
		}
		return "", ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", ErrInvalidCredentials
	}

	return s.issueToken(user)
}

// issueToken creates the normal Progo JWT for any authenticated local user.
func (s *Service) issueToken(user *User) (string, error) {
	claims := jwt.MapClaims{
		"sub":           user.ID,
		"instance_role": user.InstanceRole,
		"token_version": user.TokenVersion,
		"exp":           time.Now().Add(time.Hour * 72).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(s.jwtSecretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// OAuthProfile is normalized identity data returned by an upstream provider.
type OAuthProfile struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Username      string
	DisplayName   string
	AvatarURL     string
}

// oauthState is the signed CSRF and provider-binding payload sent through OAuth redirects.
type oauthState struct {
	Provider  string `json:"provider"`
	ExpiresAt int64  `json:"expires_at"`
	Nonce     string `json:"nonce"`
}

// userForOAuthProfile resolves or creates the local user represented by a provider profile.
func (s *Service) userForOAuthProfile(ctx context.Context, profile OAuthProfile) (*User, error) {
	profile.Provider = normalizeOAuthProvider(profile.Provider)
	profile.Email = normalizeEmail(profile.Email)
	profile.Subject = strings.TrimSpace(profile.Subject)
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.AvatarURL = strings.TrimSpace(profile.AvatarURL)
	if profile.Provider == "" || profile.Subject == "" || profile.Email == "" {
		return nil, ErrOAuthProviderFailed
	}
	if !profile.EmailVerified {
		return nil, ErrOAuthEmailNotVerified
	}
	identity, err := s.repo.GetOAuthIdentity(ctx, profile.Provider, profile.Subject)
	if err == nil {
		identity.Email = profile.Email
		identity.EmailVerified = profile.EmailVerified
		identity.DisplayName = profile.DisplayName
		identity.AvatarURL = profile.AvatarURL
		if err := s.repo.UpdateOAuthIdentity(ctx, identity); err != nil {
			return nil, err
		}
		return s.repo.GetUserByID(ctx, identity.UserID)
	}
	if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	if existing, err := s.repo.GetUserByEmail(ctx, profile.Email); err == nil && existing != nil {
		return nil, ErrOAuthEmailAlreadyRegistered
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	if !s.registrationEnabled {
		return nil, ErrRegistrationDisabled
	}
	newUser, err := s.newOAuthUser(profile)
	if err != nil {
		return nil, err
	}
	newIdentity := &OAuthIdentity{
		UserID:          newUser.ID,
		Provider:        profile.Provider,
		ProviderSubject: profile.Subject,
		Email:           profile.Email,
		EmailVerified:   profile.EmailVerified,
		DisplayName:     profile.DisplayName,
		AvatarURL:       profile.AvatarURL,
	}
	if err := s.repo.CreateUserWithOAuthIdentity(ctx, newUser, newIdentity); err != nil {
		return nil, err
	}
	newUser.PasswordHash = ""
	return newUser, nil
}

// oauthProviderConfig returns configuration for a normalized provider key.
func (s *Service) oauthProviderConfig(provider string) OAuthProviderConfig {
	switch normalizeOAuthProvider(provider) {
	case OAuthProviderGoogle:
		return s.oauthConfig.Google
	case OAuthProviderGitHub:
		return s.oauthConfig.GitHub
	default:
		return OAuthProviderConfig{}
	}
}

// configured reports whether a provider has all required OAuth settings.
func (c OAuthProviderConfig) configured() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != "" && strings.TrimSpace(c.RedirectURL) != ""
}

// normalizeOAuthProvider maps user-facing provider strings to internal provider keys.
func normalizeOAuthProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case OAuthProviderGoogle:
		return OAuthProviderGoogle
	case OAuthProviderGitHub:
		return OAuthProviderGitHub
	default:
		return ""
	}
}

// oauthURL builds a provider authorization URL with encoded query parameters.
func oauthURL(rawURL string, values map[string]string) string {
	parsed, _ := url.Parse(rawURL)
	query := parsed.Query()
	for key, value := range values {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// randomToken returns a base64url cryptographic random token.
func randomToken(byteCount int) (string, error) {
	raw := make([]byte, byteCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// hashOAuthCode returns the storage hash for a frontend exchange code.
func hashOAuthCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// oauthUsername creates a deterministic local username from provider subject data.
func oauthUsername(provider, subject string) string {
	clean := strings.ToLower(strings.TrimSpace(subject))
	var builder strings.Builder
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			builder.WriteRune(r)
		}
	}
	clean = builder.String()
	if len(clean) > 24 {
		clean = clean[:24]
	}
	if clean == "" {
		sum := sha256.Sum256([]byte(provider + ":" + subject))
		clean = hex.EncodeToString(sum[:])[:16]
	}
	return provider + "_" + clean
}

// signOAuthState serializes and authenticates an OAuth state payload.
func (s *Service) signOAuthState(state oauthState) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.jwtSecretKey)
	mac.Write([]byte(encodedPayload))
	signature := mac.Sum(nil)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// verifyOAuthState authenticates and validates an OAuth state payload.
func (s *Service) verifyOAuthState(raw string) (oauthState, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return oauthState{}, ErrOAuthInvalidState
	}
	mac := hmac.New(sha256.New, s.jwtSecretKey)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, got) {
		return oauthState{}, ErrOAuthInvalidState
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return oauthState{}, ErrOAuthInvalidState
	}
	var state oauthState
	if err := json.Unmarshal(payload, &state); err != nil {
		return oauthState{}, ErrOAuthInvalidState
	}
	if state.ExpiresAt <= time.Now().Unix() || normalizeOAuthProvider(state.Provider) == "" || state.Nonce == "" {
		return oauthState{}, ErrOAuthInvalidState
	}
	return state, nil
}

// oauthTokenResponse is the subset of provider token response fields used by login.
type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

// fetchOAuthProfile exchanges a provider code and fetches normalized identity data.
func (s *Service) fetchOAuthProfile(ctx context.Context, provider, code string) (OAuthProfile, error) {
	switch provider {
	case OAuthProviderGoogle:
		return s.fetchGoogleProfile(ctx, code)
	case OAuthProviderGitHub:
		return s.fetchGitHubProfile(ctx, code)
	default:
		return OAuthProfile{}, ErrOAuthProviderUnavailable
	}
}

// fetchGoogleProfile loads identity data from Google's OpenID Connect userinfo endpoint.
func (s *Service) fetchGoogleProfile(ctx context.Context, code string) (OAuthProfile, error) {
	cfg := s.oauthProviderConfig(OAuthProviderGoogle)
	token, err := s.exchangeOAuthToken(ctx, "https://oauth2.googleapis.com/token", map[string]string{
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
		"redirect_uri":  cfg.RedirectURL,
	})
	if err != nil {
		return OAuthProfile{}, err
	}
	var profile struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := s.getOAuthJSON(ctx, "https://openidconnect.googleapis.com/v1/userinfo", token.AccessToken, &profile); err != nil {
		return OAuthProfile{}, err
	}
	return OAuthProfile{
		Provider:      OAuthProviderGoogle,
		Subject:       profile.Subject,
		Email:         profile.Email,
		EmailVerified: profile.EmailVerified,
		DisplayName:   profile.Name,
		AvatarURL:     profile.Picture,
	}, nil
}

// fetchGitHubProfile loads identity data from GitHub's user and email APIs.
func (s *Service) fetchGitHubProfile(ctx context.Context, code string) (OAuthProfile, error) {
	cfg := s.oauthProviderConfig(OAuthProviderGitHub)
	token, err := s.exchangeOAuthToken(ctx, "https://github.com/login/oauth/access_token", map[string]string{
		"client_id":     cfg.ClientID,
		"client_secret": cfg.ClientSecret,
		"code":          code,
		"redirect_uri":  cfg.RedirectURL,
	})
	if err != nil {
		return OAuthProfile{}, err
	}
	var userInfo struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := s.getOAuthJSON(ctx, "https://api.github.com/user", token.AccessToken, &userInfo); err != nil {
		return OAuthProfile{}, err
	}
	email, verified, err := s.fetchGitHubPrimaryEmail(ctx, token.AccessToken)
	if err != nil {
		return OAuthProfile{}, err
	}
	return OAuthProfile{
		Provider:      OAuthProviderGitHub,
		Subject:       fmt.Sprintf("%d", userInfo.ID),
		Email:         email,
		EmailVerified: verified,
		Username:      userInfo.Login,
		DisplayName:   firstNonEmpty(userInfo.Name, userInfo.Login),
		AvatarURL:     userInfo.AvatarURL,
	}, nil
}

// fetchGitHubPrimaryEmail returns the verified primary email for a GitHub account.
func (s *Service) fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := s.getOAuthJSON(ctx, "https://api.github.com/user/emails", accessToken, &emails); err != nil {
		return "", false, err
	}
	for _, item := range emails {
		if item.Primary && item.Verified && strings.TrimSpace(item.Email) != "" {
			return item.Email, true, nil
		}
	}
	return "", false, ErrOAuthEmailNotVerified
}

// exchangeOAuthToken trades an authorization code for an access token.
func (s *Service) exchangeOAuthToken(ctx context.Context, endpoint string, values map[string]string) (oauthTokenResponse, error) {
	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return oauthTokenResponse{}, ErrOAuthProviderFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return oauthTokenResponse{}, ErrOAuthProviderFailed
	}
	var token oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oauthTokenResponse{}, ErrOAuthProviderFailed
	}
	if strings.TrimSpace(token.AccessToken) == "" || token.Error != "" {
		return oauthTokenResponse{}, ErrOAuthProviderFailed
	}
	return token, nil
}

// getOAuthJSON performs an authenticated provider JSON request.
func (s *Service) getOAuthJSON(ctx context.Context, endpoint, accessToken string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return ErrOAuthProviderFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return ErrOAuthProviderFailed
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return ErrOAuthProviderFailed
	}
	return nil
}

// firstNonEmpty returns the first non-blank value from a list.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

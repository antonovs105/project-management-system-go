package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidUserInput reports a malformed user-management request.
var ErrInvalidUserInput = errors.New("invalid user input")

// ErrAdminAlreadyExists reports that bootstrap cannot create another first admin.
var ErrAdminAlreadyExists = errors.New("admin user already exists")

// ErrAdminRequired reports that the current user lacks admin privileges.
var ErrAdminRequired = errors.New("admin role required")

// ErrUserNotFound reports that the target user does not exist locally.
var ErrUserNotFound = errors.New("user not found")

// ErrCannotDemoteLastAdmin protects the instance from losing its final admin.
var ErrCannotDemoteLastAdmin = errors.New("cannot demote the last admin")

// ErrInvalidCredentials reports failed authentication without revealing which credential failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

const (
	// RoleAdmin allows global administrative operations.
	RoleAdmin = "admin"
	// RoleWorker allows normal project-management operations.
	RoleWorker = "worker"

	// defaultAdminListLimit is the fallback admin user list size.
	defaultAdminListLimit = 100
	// maxAdminListLimit is the largest accepted admin user list size.
	maxAdminListLimit = 500
)

// Service encapsulates local user registration, login, and account management.
type Service struct {
	repo         Repository
	jwtSecretKey []byte
	apConfig     activitypub.Config
}

// NewService creates a user service with repository, JWT secret, and ActivityPub configuration.
func NewService(repo Repository, jwtSecret []byte, apConfig activitypub.Config) *Service {
	return &Service{
		repo:         repo,
		jwtSecretKey: jwtSecret,
		apConfig:     apConfig,
	}
}

// RegisterUser creates a local worker account and its ActivityPub actor graph.
func (s *Service) RegisterUser(ctx context.Context, username, email, password string) (*User, error) {
	newUser, err := s.newLocalUser(username, email, password, RoleWorker)
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

// BootstrapAdmin creates the first local admin account. The repository enforces
// the one-admin bootstrap guard transactionally.
func (s *Service) BootstrapAdmin(ctx context.Context, username, email, password string) (*User, error) {
	newUser, err := s.newLocalUser(username, email, password, RoleAdmin)
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
	options.Role = strings.ToLower(strings.TrimSpace(options.Role))
	if options.Role != "" && !IsValidRole(options.Role) {
		return nil, invalidUserInput("invalid user role")
	}
	options.Query = strings.TrimSpace(options.Query)
	options.Limit = normalizeListLimit(options.Limit)
	options.Offset = normalizeOffset(options.Offset)
	return s.repo.ListUsers(ctx, options)
}

// UpdateUserRole changes a local user's global role and records an audit event.
func (s *Service) UpdateUserRole(ctx context.Context, adminUserID, targetUserID, role string) (*User, error) {
	if err := s.requireAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}
	targetUserID = strings.TrimSpace(targetUserID)
	if _, err := uuid.Parse(targetUserID); err != nil {
		return nil, invalidUserInput("valid user id is required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if !IsValidRole(role) {
		return nil, invalidUserInput("invalid user role")
	}
	return s.repo.UpdateUserRole(ctx, adminUserID, targetUserID, role)
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
func (s *Service) newLocalUser(username, email, password, role string) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
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
		Role:          role,
		Handle:        activitypub.Handle(username, s.apConfig),
		Name:          username,
		Summary:       "",
		PublicKeyPEM:  publicKey,
		PrivateKeyPEM: privateKey,
	}

	return newUser, nil
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

// validatePassword enforces the local minimum password policy.
func validatePassword(password string) error {
	if len(password) < 8 {
		return invalidUserInput("password must be at least 8 characters")
	}
	return nil
}

// IsValidRole reports whether role is a supported system-wide user role.
func IsValidRole(role string) bool {
	return role == RoleAdmin || role == RoleWorker
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

// requireAdmin verifies that userID belongs to a global admin account.
func (s *Service) requireAdmin(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAdminRequired
	}
	role, err := s.repo.UserRole(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAdminRequired
		}
		return err
	}
	if role != RoleAdmin {
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
	email = strings.TrimSpace(email)

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

	claims := jwt.MapClaims{
		"sub":           user.ID,
		"role":          user.Role,
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

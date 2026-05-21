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

var ErrInvalidUserInput = errors.New("invalid user input")
var ErrAdminAlreadyExists = errors.New("admin user already exists")
var ErrAdminRequired = errors.New("admin role required")
var ErrUserNotFound = errors.New("user not found")
var ErrCannotDemoteLastAdmin = errors.New("cannot demote the last admin")

const (
	RoleAdmin  = "admin"
	RoleWorker = "worker"
)

// Service incapsulates business logic for working with users
// Depends on repository for data access
type Service struct {
	repo         Repository
	jwtSecretKey []byte
	apConfig     activitypub.Config
}

// constructor for UserService.
func NewService(repo Repository, jwtSecret []byte, apConfig activitypub.Config) *Service {
	return &Service{
		repo:         repo,
		jwtSecretKey: jwtSecret,
		apConfig:     apConfig,
	}
}

// RegisterUser - service method for user registration
// Hashing password and adds user via repository
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

func (s *Service) ListUsers(ctx context.Context, adminUserID string) ([]User, error) {
	if err := s.requireAdmin(ctx, adminUserID); err != nil {
		return nil, err
	}
	return s.repo.ListUsers(ctx)
}

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

func (s *Service) newLocalUser(username, email, password, role string) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if err := validateRegistrationInput(username, email, password); err != nil {
		return nil, err
	}

	// Hashing password
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

	// Creating struct User
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
	if len(password) < 8 {
		return invalidUserInput("password must be at least 8 characters")
	}
	return nil
}

func IsValidRole(role string) bool {
	return role == RoleAdmin || role == RoleWorker
}

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

func invalidUserInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidUserInput, message)
}

// Login checks users and returns JWT
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	// searching user in DB
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		log.Printf("[DEBUG] Login failed for email '%s'. Reason: user not found or DB error. Error: %v", email, err)
		return "", errors.New("invalid credentials")
	}

	// comparing hashes
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", errors.New("invalid credentials")
	}

	// generating JWT
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  time.Now().Add(time.Hour * 72).Unix(),
	}

	// creating new token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Signing token
	tokenString, err := token.SignedString(s.jwtSecretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

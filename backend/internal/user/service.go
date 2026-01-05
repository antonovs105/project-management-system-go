package user

import (
	"context"
	"log"

	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Service incapsulates business logic for working with users
// Depends on repository for data access
type Service struct {
	repo         Repository
	jwtSecretKey []byte
}

// constructor for UserService.
func NewService(repo Repository, jwtSecret []byte) *Service {
	return &Service{
		repo:         repo,
		jwtSecretKey: jwtSecret,
	}
}

// RegisterUser - service method for user registration
// Hashing password and adds user via repository
func (s *Service) RegisterUser(ctx context.Context, username, email, password string) (*User, error) {
	// Hashing password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Creating struct User
	newUser := &User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
	}

	// Saving User in DB
	// Calling repository method for INSERT-query
	err = s.repo.CreateUser(ctx, newUser)
	if err != nil {
		// TODO: error handling
		// for now just returning error
		return nil, err
	}

	// returning created User and clearing password hash
	newUser.PasswordHash = ""

	return newUser, nil
}

// Login checks users and returns JWT
func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	// searching user in DB
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		log.Printf("[DEBUG] Login failed for email '%s'. Reason: user not found or DB error. Error: %v", email, err)
		return "", errors.New("invalid credentials")
	}

	log.Println("---------------------------------")
	log.Printf("[DEBUG] Attempting login for user ID: %d", user.ID)
	log.Printf("[DEBUG] Email from request: '%s'", email)
	log.Printf("[DEBUG] Password from request: '%s'", password)
	log.Printf("[DEBUG] Hash from DB: '%s'", user.PasswordHash)
	log.Println("---------------------------------")

	// comparing hashes
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		log.Printf("[DEBUG] Password comparison failed. Reason: %v", err)
		return "", errors.New("invalid credentials")
	}

	log.Printf("[DEBUG] Password for user ID %d comparison successful!", user.ID)

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

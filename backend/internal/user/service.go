package user

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

// Service incapsulates business logic for working with users
// Depends on repository for data access
type Service struct {
	repo *Repository
}

// constructor for UserService.
func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
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

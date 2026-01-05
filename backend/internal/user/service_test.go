package user

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func TestService_RegisterUser(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, []byte("secret"))

	ctx := context.Background()
	username := "testuser"
	email := "test@example.com"
	password := "password123"

	// Success case
	t.Run("Success", func(t *testing.T) {
		mockRepo.On("CreateUser", ctx, mock.AnythingOfType("*user.User")).Return(nil).Once()

		user, err := service.RegisterUser(ctx, username, email, password)

		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, username, user.Username)
		assert.Equal(t, email, user.Email)
		assert.Empty(t, user.PasswordHash) // Password hash should be cleared
		mockRepo.AssertExpectations(t)
	})

	// Repository error case
	t.Run("RepositoryError", func(t *testing.T) {
		repoErr := errors.New("db error")
		mockRepo.On("CreateUser", ctx, mock.AnythingOfType("*user.User")).Return(repoErr).Once()

		user, err := service.RegisterUser(ctx, username, email, password)

		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Equal(t, repoErr, err)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_Login(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, []byte("secret"))

	ctx := context.Background()
	email := "test@example.com"
	password := "password123"

	// Setup a user with hashed password
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	existingUser := &User{
		ID:           1,
		Username:     "testuser",
		Email:        email,
		PasswordHash: string(hashedPassword),
		Role:         "user",
	}

	// Success case
	t.Run("Success", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, email).Return(existingUser, nil).Once()

		token, err := service.Login(ctx, email, password)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
		mockRepo.AssertExpectations(t)
	})

	// User not found
	t.Run("UserNotFound", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, email).Return(nil, errors.New("user not found")).Once()

		token, err := service.Login(ctx, email, password)

		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Equal(t, "invalid credentials", err.Error())
		mockRepo.AssertExpectations(t)
	})

	// Wrong password
	t.Run("WrongPassword", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, email).Return(existingUser, nil).Once()

		token, err := service.Login(ctx, email, "wrongpassword")

		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Equal(t, "invalid credentials", err.Error())
		mockRepo.AssertExpectations(t)
	})
}

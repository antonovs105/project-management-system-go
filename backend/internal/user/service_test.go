package user

import (
	"context"
	"errors"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func TestService_RegisterUser(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, []byte("secret"), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

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
		assert.Equal(t, RoleWorker, user.Role)
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

	t.Run("InvalidInput", func(t *testing.T) {
		user, err := service.RegisterUser(ctx, "bad/name", email, password)

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		assert.Nil(t, user)
	})
}

func TestService_BootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("CreateAdminIfNoAdmin", ctx, mock.AnythingOfType("*user.User")).Return(nil).Once()

		admin, err := service.BootstrapAdmin(ctx, "admin", "admin@example.com", "password123")

		assert.NoError(t, err)
		assert.NotNil(t, admin)
		assert.Equal(t, "admin", admin.Username)
		assert.Equal(t, RoleAdmin, admin.Role)
		assert.Empty(t, admin.PasswordHash)
		assert.NotEmpty(t, admin.PublicKeyPEM)
		assert.NotEmpty(t, admin.PrivateKeyPEM)
		mockRepo.AssertExpectations(t)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("CreateAdminIfNoAdmin", ctx, mock.AnythingOfType("*user.User")).Return(ErrAdminAlreadyExists).Once()

		admin, err := service.BootstrapAdmin(ctx, "admin", "admin@example.com", "password123")

		assert.ErrorIs(t, err, ErrAdminAlreadyExists)
		assert.Nil(t, admin)
		mockRepo.AssertExpectations(t)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)

		admin, err := service.BootstrapAdmin(ctx, "bad/name", "admin@example.com", "password123")

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		assert.Nil(t, admin)
		mockRepo.AssertNotCalled(t, "CreateAdminIfNoAdmin")
	})
}

func TestService_AdminUserManagement(t *testing.T) {
	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	adminID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"

	t.Run("ListUsers", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		users := []User{{ID: adminID, Role: RoleAdmin}, {ID: targetID, Role: RoleWorker}}
		mockRepo.On("UserRole", ctx, adminID).Return(RoleAdmin, nil).Once()
		mockRepo.On("ListUsers", ctx, ListUsersOptions{
			Role:   RoleAdmin,
			Query:  "adm",
			Limit:  maxAdminListLimit,
			Offset: 25,
		}).Return(users, nil).Once()

		got, err := service.ListUsers(ctx, adminID, ListUsersOptions{
			Role:   " ADMIN ",
			Query:  " adm ",
			Limit:  1000,
			Offset: 25,
		})

		assert.NoError(t, err)
		assert.Equal(t, users, got)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RequiresAdmin", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("UserRole", ctx, targetID).Return(RoleWorker, nil).Once()

		got, err := service.ListUsers(ctx, targetID, ListUsersOptions{})

		assert.ErrorIs(t, err, ErrAdminRequired)
		assert.Nil(t, got)
		mockRepo.AssertNotCalled(t, "ListUsers")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsInvalidListFilter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("UserRole", ctx, adminID).Return(RoleAdmin, nil).Once()

		got, err := service.ListUsers(ctx, adminID, ListUsersOptions{Role: "owner"})

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		assert.Nil(t, got)
		mockRepo.AssertNotCalled(t, "ListUsers")
		mockRepo.AssertExpectations(t)
	})

	t.Run("UpdateRole", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		updated := &User{ID: targetID, Role: RoleAdmin}
		mockRepo.On("UserRole", ctx, adminID).Return(RoleAdmin, nil).Once()
		mockRepo.On("UpdateUserRole", ctx, adminID, targetID, RoleAdmin).Return(updated, nil).Once()

		got, err := service.UpdateUserRole(ctx, adminID, targetID, " ADMIN ")

		assert.NoError(t, err)
		assert.Equal(t, updated, got)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsInvalidInput", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("UserRole", ctx, adminID).Return(RoleAdmin, nil).Once()

		got, err := service.UpdateUserRole(ctx, adminID, targetID, "owner")

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		assert.Nil(t, got)
		mockRepo.AssertNotCalled(t, "UpdateUserRole")
		mockRepo.AssertExpectations(t)
	})
}

func TestService_ChangePassword(t *testing.T) {
	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userID := "11111111-1111-4111-8111-111111111111"
	oldPassword := "password123"
	newPassword := "newpassword123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.DefaultCost)
	assert.NoError(t, err)

	t.Run("Success", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("GetUserByID", ctx, userID).Return(&User{ID: userID, PasswordHash: string(hashedPassword)}, nil).Once()
		mockRepo.On("UpdatePasswordHash", ctx, userID, mock.MatchedBy(func(hash string) bool {
			return bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)) == nil
		})).Return(nil).Once()

		err := service.ChangePassword(ctx, userID, oldPassword, newPassword)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsWrongCurrentPassword", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("GetUserByID", ctx, userID).Return(&User{ID: userID, PasswordHash: string(hashedPassword)}, nil).Once()

		err := service.ChangePassword(ctx, userID, "wrongpassword", newPassword)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
		mockRepo.AssertNotCalled(t, "UpdatePasswordHash")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsSamePassword", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("GetUserByID", ctx, userID).Return(&User{ID: userID, PasswordHash: string(hashedPassword)}, nil).Once()

		err := service.ChangePassword(ctx, userID, oldPassword, oldPassword)

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		mockRepo.AssertNotCalled(t, "UpdatePasswordHash")
		mockRepo.AssertExpectations(t)
	})

	t.Run("RejectsWeakNewPassword", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)

		err := service.ChangePassword(ctx, userID, oldPassword, "short")

		assert.ErrorIs(t, err, ErrInvalidUserInput)
		mockRepo.AssertNotCalled(t, "GetUserByID")
		mockRepo.AssertNotCalled(t, "UpdatePasswordHash")
	})
}

func TestService_ValidateTokenVersion(t *testing.T) {
	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userID := "11111111-1111-4111-8111-111111111111"

	t.Run("CurrentVersion", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("TokenVersion", ctx, userID).Return(3, nil).Once()

		err := service.ValidateTokenVersion(ctx, userID, 3)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("StaleVersion", func(t *testing.T) {
		mockRepo := new(MockRepository)
		service := NewService(mockRepo, []byte("secret"), cfg)
		mockRepo.On("TokenVersion", ctx, userID).Return(4, nil).Once()

		err := service.ValidateTokenVersion(ctx, userID, 3)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
		mockRepo.AssertExpectations(t)
	})
}

func TestService_Login(t *testing.T) {
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, []byte("secret"), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	ctx := context.Background()
	email := "test@example.com"
	password := "password123"

	// Setup a user with hashed password
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	existingUser := &User{
		ID:           "user-1",
		Username:     "testuser",
		Email:        email,
		PasswordHash: string(hashedPassword),
		Role:         "user",
		TokenVersion: 2,
	}

	// Success case
	t.Run("Success", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, email).Return(existingUser, nil).Once()

		token, err := service.Login(ctx, email, password)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
		claims := parseTokenClaims(t, token, []byte("secret"))
		assert.Equal(t, float64(2), claims["token_version"])
		mockRepo.AssertExpectations(t)
	})

	t.Run("NormalizesEmail", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, email).Return(existingUser, nil).Once()

		token, err := service.Login(ctx, " Test@Example.COM ", password)

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

func parseTokenClaims(t *testing.T, raw string, secret []byte) jwt.MapClaims {
	t.Helper()

	parsed, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		return secret, nil
	})
	assert.NoError(t, err)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	return claims
}

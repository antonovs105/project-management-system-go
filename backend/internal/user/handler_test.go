package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHandler_AdminUserRoutes(t *testing.T) {
	adminID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"

	t.Run("ListUsers", func(t *testing.T) {
		repo := new(MockRepository)
		users := []User{{ID: adminID, InstanceRole: InstanceRoleAdmin}, {ID: targetID, InstanceRole: InstanceRoleUser}}
		repo.On("InstanceRole", mock.Anything, adminID).Return(InstanceRoleAdmin, nil).Once()
		repo.On("ListUsers", mock.Anything, ListUsersOptions{
			InstanceRole: InstanceRoleUser,
			Query:        "work",
			Limit:        10,
			Offset:       20,
		}).Return(users, nil).Once()
		e := newAdminUserEcho(repo, adminID)

		rec := doAdminUserRequest(e, http.MethodGet, "/api/admin/users?role=user&q=work&limit=10&offset=20", "")

		require.Equal(t, http.StatusOK, rec.Code)
		var body []User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body, 2)
		assert.Equal(t, InstanceRoleAdmin, body[0].InstanceRole)
		repo.AssertExpectations(t)
	})

	t.Run("RejectsNonAdmin", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("InstanceRole", mock.Anything, targetID).Return(InstanceRoleUser, nil).Once()
		e := newAdminUserEcho(repo, targetID)

		rec := doAdminUserRequest(e, http.MethodGet, "/api/admin/users", "")

		require.Equal(t, http.StatusForbidden, rec.Code)
		repo.AssertExpectations(t)
	})

	t.Run("UpdatesRole", func(t *testing.T) {
		repo := new(MockRepository)
		updated := &User{ID: targetID, InstanceRole: InstanceRoleAdmin}
		repo.On("InstanceRole", mock.Anything, adminID).Return(InstanceRoleAdmin, nil).Once()
		repo.On("UpdateInstanceRole", mock.Anything, adminID, targetID, InstanceRoleAdmin).Return(updated, nil).Once()
		e := newAdminUserEcho(repo, adminID)

		rec := doAdminUserRequest(e, http.MethodPatch, "/api/admin/users/"+targetID+"/role", `{"instance_role":"admin"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		var body User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, InstanceRoleAdmin, body.InstanceRole)
		repo.AssertExpectations(t)
	})
}

func TestHandler_ChangePassword(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	oldPassword := "password123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.DefaultCost)
	require.NoError(t, err)

	t.Run("ChangesPassword", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetUserByID", mock.Anything, userID).Return(&User{ID: userID, PasswordHash: string(hashedPassword)}, nil).Once()
		repo.On("UpdatePasswordHash", mock.Anything, userID, mock.AnythingOfType("string")).Return(nil).Once()
		e := newAccountEcho(repo, userID)

		rec := doAdminUserRequest(e, http.MethodPatch, "/api/me/password", `{"current_password":"password123","new_password":"newpassword123"}`)

		require.Equal(t, http.StatusNoContent, rec.Code)
		repo.AssertExpectations(t)
	})

	t.Run("MapsInvalidCurrentPassword", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetUserByID", mock.Anything, userID).Return(&User{ID: userID, PasswordHash: string(hashedPassword)}, nil).Once()
		e := newAccountEcho(repo, userID)

		rec := doAdminUserRequest(e, http.MethodPatch, "/api/me/password", `{"current_password":"wrongpassword","new_password":"newpassword123"}`)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		repo.AssertExpectations(t)
	})

	t.Run("RejectsWeakPassword", func(t *testing.T) {
		e := newAccountEcho(new(MockRepository), userID)

		rec := doAdminUserRequest(e, http.MethodPatch, "/api/me/password", `{"current_password":"password123","new_password":"short"}`)

		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("HidesRepositoryError", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("GetUserByID", mock.Anything, userID).Return(nil, errors.New("db down")).Once()
		e := newAccountEcho(repo, userID)

		rec := doAdminUserRequest(e, http.MethodPatch, "/api/me/password", `{"current_password":"password123","new_password":"newpassword123"}`)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		repo.AssertExpectations(t)
	})
}

func newAdminUserEcho(repo Repository, userID string) *echo.Echo {
	e := echo.New()
	api := e.Group("/api")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userID", userID)
			return next(c)
		}
	})
	service := NewService(repo, []byte("secret"), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	NewHandler(service).RegisterAdminRoutes(api)
	return e
}

func newAccountEcho(repo Repository, userID string) *echo.Echo {
	e := echo.New()
	api := e.Group("/api")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userID", userID)
			return next(c)
		}
	})
	service := NewService(repo, []byte("secret"), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	NewHandler(service).RegisterAccountRoutes(api)
	return e
}

func doAdminUserRequest(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

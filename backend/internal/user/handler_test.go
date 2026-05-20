package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandler_BootstrapAdmin(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		e := newBootstrapEcho(new(MockRepository), "")

		rec := postBootstrapAdmin(e, "ignored", `{"username":"admin","email":"admin@example.com","password":"password123"}`)

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("RejectsInvalidToken", func(t *testing.T) {
		e := newBootstrapEcho(new(MockRepository), "correct-token")

		rec := postBootstrapAdmin(e, "wrong-token", `{"username":"admin","email":"admin@example.com","password":"password123"}`)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("CreatesAdmin", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("CreateAdminIfNoAdmin", mock.Anything, mock.AnythingOfType("*user.User")).Return(nil).Once()
		e := newBootstrapEcho(repo, "correct-token")

		rec := postBootstrapAdmin(e, "correct-token", `{"username":"admin","email":"admin@example.com","password":"password123"}`)

		require.Equal(t, http.StatusCreated, rec.Code)
		var body User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "admin", body.Username)
		assert.Equal(t, RoleAdmin, body.Role)
		assert.Empty(t, body.PasswordHash)
		repo.AssertExpectations(t)
	})

	t.Run("MapsAlreadyExists", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("CreateAdminIfNoAdmin", mock.Anything, mock.AnythingOfType("*user.User")).Return(ErrAdminAlreadyExists).Once()
		e := newBootstrapEcho(repo, "correct-token")

		rec := postBootstrapAdmin(e, "correct-token", `{"username":"admin","email":"admin@example.com","password":"password123"}`)

		require.Equal(t, http.StatusConflict, rec.Code)
		repo.AssertExpectations(t)
	})
}

func newBootstrapEcho(repo Repository, token string) *echo.Echo {
	e := echo.New()
	service := NewService(repo, []byte("secret"), activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	NewHandler(service, token).RegisterRoutes(e)
	return e
}

func postBootstrapAdmin(e *echo.Echo, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/setup/admin", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if token != "" {
		req.Header.Set(AdminBootstrapTokenHeader, token)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

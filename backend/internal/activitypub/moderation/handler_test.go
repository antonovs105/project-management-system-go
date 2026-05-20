package moderation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerListsDomainBlocks(t *testing.T) {
	creator := "admin-1"
	repo := &fakeRepository{
		role: RoleAdmin,
		blocks: []DomainBlock{
			{
				ID:        "block-1",
				Domain:    "remote.example",
				Reason:    "spam",
				CreatedBy: &creator,
				CreatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	e := newModerationHandlerEcho(repo, "admin-1")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/federation/domain-blocks", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response []DomainBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, "remote.example", response[0].Domain)
}

func TestHandlerBlocksDomain(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	e := newModerationHandlerEcho(repo, "admin-1")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/federation/domain-blocks", strings.NewReader(`{"domain":"Remote.Example","reason":"spam"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "remote.example", repo.upsertDomain)
	assert.JSONEq(t, `{"id":"block-1","domain":"remote.example","reason":"spam","created_by":"admin-1","created_at":"2026-05-21T12:00:00Z","updated_at":"2026-05-21T12:00:00Z"}`, rec.Body.String())
}

func TestHandlerRejectsNonAdmin(t *testing.T) {
	e := newModerationHandlerEcho(&fakeRepository{role: "worker"}, "user-1")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/federation/domain-blocks", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"admin permissions required"}`, rec.Body.String())
}

func TestHandlerUnblocksDomain(t *testing.T) {
	repo := &fakeRepository{role: RoleAdmin}
	e := newModerationHandlerEcho(repo, "admin-1")
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/federation/domain-blocks/Remote.Example", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "remote.example", repo.deleteDomain)
}

func newModerationHandlerEcho(repo *fakeRepository, userID string) *echo.Echo {
	e := echo.New()
	api := e.Group("/api")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userID", userID)
			return next(c)
		}
	})
	NewHandler(NewService(repo)).RegisterRoutes(api)
	return e
}

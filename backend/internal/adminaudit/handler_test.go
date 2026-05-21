package adminaudit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerListsAuditEvents(t *testing.T) {
	repo := &fakeRepository{
		role: instanceRoleAdmin,
		events: []Event{
			{
				ID:         "event-1",
				Action:     ActionUserInstanceRoleUpdated,
				TargetType: TargetTypeUser,
				TargetID:   "user-1",
				Metadata:   json.RawMessage(`{"old_instance_role":"user","new_instance_role":"admin"}`),
			},
		},
	}
	e := newHandlerEcho(repo, "admin-1")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-events?action=user.instance_role_updated&target_type=user&limit=10&offset=20", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, ActionUserInstanceRoleUpdated, repo.options.Action)
	assert.Equal(t, TargetTypeUser, repo.options.TargetType)
	assert.Equal(t, 10, repo.options.Limit)
	assert.Equal(t, 20, repo.options.Offset)

	var response []Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, "event-1", response[0].ID)
	assert.JSONEq(t, `{"old_instance_role":"user","new_instance_role":"admin"}`, string(response[0].Metadata))
}

func TestHandlerRejectsNonAdmin(t *testing.T) {
	e := newHandlerEcho(&fakeRepository{role: "user"}, "user-1")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-events", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"admin privileges required"}`, rec.Body.String())
}

func TestHandlerRejectsInvalidLimit(t *testing.T) {
	e := newHandlerEcho(&fakeRepository{role: instanceRoleAdmin}, "admin-1")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit-events?limit=never", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"invalid admin audit filter"}`, rec.Body.String())
}

func newHandlerEcho(repo Repository, userID string) *echo.Echo {
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

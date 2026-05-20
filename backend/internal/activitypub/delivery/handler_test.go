package delivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerListsProjectDeliveries(t *testing.T) {
	repo := &serviceRepo{
		projectDeliveries: []ProjectDelivery{
			{
				ID:             "delivery-1",
				ActivityAPID:   "https://local.example/activities/1",
				ActivityType:   "Create",
				TargetInboxURL: "https://remote.example/inbox",
				State:          StateFailed,
				Attempts:       2,
				MaxAttempts:    10,
			},
		},
	}
	e := newDeliveryHandlerEcho(repo, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-1/deliveries", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "project-1", repo.projectID)
	assert.Equal(t, "user-1", repo.userID)

	var response []ProjectDelivery
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response, 1)
	assert.Equal(t, "delivery-1", response[0].ID)
	assert.Equal(t, "https://local.example/activities/1", response[0].ActivityAPID)
	assert.Equal(t, StateFailed, response[0].State)
}

func TestHandlerRejectsProjectDeliveryAccessDenied(t *testing.T) {
	repo := &serviceRepo{err: ErrProjectAccessDenied}
	e := newDeliveryHandlerEcho(repo, "outsider")

	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-1/deliveries", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"project not found or access denied"}`, rec.Body.String())
}

func TestHandlerHidesProjectDeliveryInternalErrors(t *testing.T) {
	repo := &serviceRepo{err: errors.New("database failed near private_key_pem")}
	e := newDeliveryHandlerEcho(repo, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-1/deliveries", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"failed to list project deliveries"}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "private_key_pem")
}

func TestHandlerRetriesProjectDelivery(t *testing.T) {
	repo := &serviceRepo{
		retryDelivery: &Delivery{
			ID:          "delivery-1",
			State:       StatePending,
			Attempts:    0,
			MaxAttempts: DefaultMaxRetry,
		},
	}
	e := newDeliveryHandlerEcho(repo, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "project-1", repo.projectID)
	assert.Equal(t, "owner-1", repo.userID)
	assert.Equal(t, "delivery-1", repo.retryDeliveryID)

	var response Delivery
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "delivery-1", response.ID)
	assert.Equal(t, StatePending, response.State)
}

func TestHandlerRejectsRetryWithoutProjectPermission(t *testing.T) {
	repo := &serviceRepo{retryErr: ErrDeliveryRetryDenied}
	e := newDeliveryHandlerEcho(repo, "viewer-1")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"insufficient permissions to retry delivery"}`, rec.Body.String())
}

func TestHandlerMapsMissingRetryDeliveryToNotFound(t *testing.T) {
	repo := &serviceRepo{retryErr: ErrDeliveryNotFound}
	e := newDeliveryHandlerEcho(repo, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/deliveries/missing/retry", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"activity delivery not found"}`, rec.Body.String())
}

func TestHandlerMapsRetryUnavailableToConflict(t *testing.T) {
	repo := &serviceRepo{retryErr: ErrDeliveryRetryUnavailable}
	e := newDeliveryHandlerEcho(repo, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.JSONEq(t, `{"error":"activity delivery cannot be retried"}`, rec.Body.String())
}

func TestHandlerHidesRetryInternalErrors(t *testing.T) {
	repo := &serviceRepo{retryErr: errors.New("database failed near private_key_pem")}
	e := newDeliveryHandlerEcho(repo, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/project-1/deliveries/delivery-1/retry", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"failed to retry project delivery"}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "private_key_pem")
}

func newDeliveryHandlerEcho(repo *serviceRepo, userID string) *echo.Echo {
	e := echo.New()
	api := e.Group("/api")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("userID", userID)
			return next(c)
		}
	})
	NewHandler(NewService(repo, &serviceQueue{})).RegisterRoutes(api)
	return e
}

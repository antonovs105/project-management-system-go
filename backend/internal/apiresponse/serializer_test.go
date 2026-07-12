package apiresponse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestSerializerWrapsLegacyErrorMaps(t *testing.T) {
	e := echo.New()
	e.JSONSerializer = Serializer{}
	e.GET("/failure", func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderXRequestID, "request-123")
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token", "code": "auth_invalid"})
	})
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"auth_invalid","message":"invalid token","request_id":"request-123"}}`, recorder.Body.String())
}

func TestHTTPErrorHandlerHidesInternalFailures(t *testing.T) {
	e := echo.New()
	e.JSONSerializer = Serializer{}
	e.HTTPErrorHandler = HTTPErrorHandler
	e.GET("/failure", func(echo.Context) error { return errors.New("database password leaked") })
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "database password")
	require.Contains(t, recorder.Body.String(), "internal server error")
}

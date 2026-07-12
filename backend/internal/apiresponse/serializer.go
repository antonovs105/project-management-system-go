// Package apiresponse centralizes the public HTTP error contract.
package apiresponse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Error is the stable machine-readable public failure payload.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorEnvelope wraps every runtime JSON error response.
type ErrorEnvelope struct {
	Error Error `json:"error"`
}

// Serializer wraps legacy handler maps into the uniform public envelope while
// leaving successful response DTOs unchanged.
type Serializer struct{}

// Serialize implements echo.JSONSerializer.
func (Serializer) Serialize(c echo.Context, value any, indent string) error {
	if message, code, ok := legacyError(value); ok {
		value = ErrorEnvelope{Error: Error{
			Code:      normalizedCode(code, c.Response().Status),
			Message:   message,
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		}}
	}
	encoder := json.NewEncoder(c.Response())
	if indent != "" {
		encoder.SetIndent("", indent)
	}
	return encoder.Encode(value)
}

// Deserialize implements echo.JSONSerializer with a single JSON value per request.
func (Serializer) Deserialize(c echo.Context, value any) error {
	err := json.NewDecoder(c.Request().Body).Decode(value)
	if err == io.EOF {
		return echo.NewHTTPError(http.StatusBadRequest, "request body is required").SetInternal(err)
	}
	return err
}

// HTTPErrorHandler converts framework and middleware failures through the same serializer.
func HTTPErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status := http.StatusInternalServerError
	message := "internal server error"
	if value, ok := err.(*echo.HTTPError); ok {
		status = value.Code
		switch detail := value.Message.(type) {
		case string:
			message = detail
		case error:
			message = detail.Error()
		default:
			message = fmt.Sprint(detail)
		}
	}
	if status >= http.StatusInternalServerError {
		c.Logger().Error(err)
		message = "internal server error"
	}
	_ = c.JSON(status, map[string]string{"error": message})
}

// legacyError recognizes existing handler responses during the envelope migration.
func legacyError(value any) (string, string, bool) {
	switch payload := value.(type) {
	case map[string]string:
		message, ok := payload["error"]
		return message, payload["code"], ok
	case map[string]any:
		message, ok := payload["error"].(string)
		code, _ := payload["code"].(string)
		return message, code, ok
	default:
		return "", "", false
	}
}

// normalizedCode supplies a stable status-derived fallback code.
func normalizedCode(code string, status int) string {
	code = strings.TrimSpace(code)
	if code != "" {
		return code
	}
	if status < 400 {
		status = http.StatusInternalServerError
	}
	return fmt.Sprintf("http_%d", status)
}

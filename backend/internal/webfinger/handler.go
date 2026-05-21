package webfinger

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// jrdMediaType is the JSON Resource Descriptor response media type.
const jrdMediaType = "application/jrd+json"

// Handler exposes the WebFinger discovery endpoint.
type Handler struct {
	service *Service
}

// NewHandler creates a WebFinger HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers the WebFinger well-known route.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/.well-known/webfinger", h.Resolve)
}

// Resolve handles RFC 7033 resource lookup requests.
func (h *Handler) Resolve(c echo.Context) error {
	if !acceptsJRDResponse(c.Request().Header.Get(echo.HeaderAccept)) {
		return c.JSON(http.StatusNotAcceptable, map[string]string{"error": "accept header must allow webfinger json"})
	}

	jrd, err := h.service.Resolve(c.Request().Context(), c.QueryParam("resource"))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResource):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve webfinger resource"})
		}
	}

	raw, err := json.Marshal(jrd)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode webfinger response"})
	}
	return c.Blob(http.StatusOK, jrdMediaType, raw)
}

// acceptsJRDResponse reports whether an Accept header allows WebFinger JSON.
func acceptsJRDResponse(header string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			mediaType = strings.TrimSpace(strings.Split(part, ";")[0])
			params = nil
		}
		if isZeroQuality(params["q"]) {
			continue
		}
		switch strings.ToLower(mediaType) {
		case "*/*", "application/*", "application/json", jrdMediaType:
			return true
		}
	}
	return false
}

// isZeroQuality reports whether an Accept q-value explicitly disables a media type.
func isZeroQuality(raw string) bool {
	if raw == "" {
		return false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return err == nil && value <= 0
}

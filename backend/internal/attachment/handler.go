package attachment

import (
	"errors"
	"fmt"
	"mime"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler exposes ticket attachment HTTP endpoints.
type Handler struct{ service *Service }

// NewHandler returns an attachment handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterRoutes mounts authenticated attachment routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/tickets/:ticketID/attachments", h.List)
	api.POST("/tickets/:ticketID/attachments", h.Upload)
	api.GET("/attachments/:attachmentID/content", h.Download)
	api.DELETE("/attachments/:attachmentID", h.Delete)
}

// Upload stores one multipart field named file.
func (h *Handler) Upload(c echo.Context) error {
	if !validID(c.Param("ticketID")) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid ticket ID"})
	}
	header, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file is required"})
	}
	if header.Size < 1 || header.Size > MaxSizeBytes {
		return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error": "attachment must be between 1 byte and 10 MiB"})
	}
	source, err := header.Open()
	if err != nil {
		return writeAttachmentError(c, err)
	}
	defer source.Close()
	userID, _ := c.Get("userID").(string)
	value, err := h.service.Upload(c.Request().Context(), c.Param("ticketID"), userID, header.Filename, source)
	if err != nil {
		return writeAttachmentError(c, err)
	}
	return c.JSON(http.StatusCreated, value)
}

// List returns attachment metadata for one ticket.
func (h *Handler) List(c echo.Context) error {
	if !validID(c.Param("ticketID")) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid ticket ID"})
	}
	userID, _ := c.Get("userID").(string)
	values, err := h.service.List(c.Request().Context(), c.Param("ticketID"), userID)
	if err != nil {
		return writeAttachmentError(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

// Download streams an attachment as a forced download.
func (h *Handler) Download(c echo.Context) error {
	if !validID(c.Param("attachmentID")) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid attachment ID"})
	}
	userID, _ := c.Get("userID").(string)
	value, reader, err := h.service.Open(c.Request().Context(), c.Param("attachmentID"), userID)
	if err != nil {
		return writeAttachmentError(c, err)
	}
	defer reader.Close()
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": value.Filename}))
	c.Response().Header().Set("X-Content-Type-Options", "nosniff")
	c.Response().Header().Set("Content-Length", fmt.Sprint(value.SizeBytes))
	return c.Stream(http.StatusOK, value.ContentType, reader)
}

// Delete removes one attachment.
func (h *Handler) Delete(c echo.Context) error {
	if !validID(c.Param("attachmentID")) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid attachment ID"})
	}
	userID, _ := c.Get("userID").(string)
	if err := h.service.Delete(c.Request().Context(), c.Param("attachmentID"), userID); err != nil {
		return writeAttachmentError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// validID reports whether a route parameter is a canonical UUID value.
func validID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// writeAttachmentError maps attachment failures.
func writeAttachmentError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrInvalidFile), errors.Is(err, ErrInfected):
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "attachment operation failed"})
	}
}

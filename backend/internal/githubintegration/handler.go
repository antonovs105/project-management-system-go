package githubintegration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// Handler exposes GitHub integration HTTP endpoints.
type Handler struct {
	service       *Service
	webhookSecret string
}

// HandlerOption customizes the GitHub HTTP adapter.
type HandlerOption func(*Handler)

// WithWebhookSecret configures GitHub webhook HMAC verification.
func WithWebhookSecret(secret string) HandlerOption {
	return func(h *Handler) {
		h.webhookSecret = strings.TrimSpace(secret)
	}
}

// NewHandler creates a GitHub integration HTTP handler.
func NewHandler(service *Service, options ...HandlerOption) *Handler {
	handler := &Handler{service: service}
	for _, option := range options {
		option(handler)
	}
	return handler
}

// RegisterRoutes registers authenticated GitHub integration routes.
func (h *Handler) RegisterRoutes(api *echo.Group) {
	api.GET("/projects/:projectID/github/repositories", h.ListRepositories)
	api.POST("/projects/:projectID/github/repositories", h.LinkRepository)
	api.DELETE("/projects/:projectID/github/repositories/:repositoryID", h.DeleteRepository)
	api.POST("/projects/:projectID/github/repositories/:repositoryID/sync", h.SyncRepository)
	api.GET("/projects/:projectID/github/commits", h.ListProjectCommits)
	api.GET("/tickets/:ticketID/github/commits", h.ListTicketCommits)
	api.POST("/tickets/:ticketID/github/commits", h.LinkCommitToTicket)
	api.DELETE("/tickets/:ticketID/github/commits/:commitID", h.UnlinkCommitFromTicket)
}

// RegisterWebhookRoutes registers public HMAC-verified GitHub webhook routes.
func (h *Handler) RegisterWebhookRoutes(e *echo.Echo, middleware ...echo.MiddlewareFunc) {
	e.POST("/webhooks/github", h.HandleWebhook, middleware...)
}

// linkRepositoryRequest is the JSON payload for attaching a GitHub repository.
type linkRepositoryRequest struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// linkCommitRequest is the JSON payload for manually attaching a commit to a ticket.
type linkCommitRequest struct {
	CommitID string `json:"commit_id"`
}

// ListRepositories returns GitHub repositories attached to a project.
func (h *Handler) ListRepositories(c echo.Context) error {
	repos, err := h.service.ListRepositories(c.Request().Context(), c.Param("projectID"), currentUserID(c))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusOK, repos)
}

// LinkRepository attaches a GitHub repository to a project.
func (h *Handler) LinkRepository(c echo.Context) error {
	var req linkRepositoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	repo, err := h.service.LinkRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), req.Owner, req.Name)
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusCreated, repo)
}

// DeleteRepository removes a project GitHub repository link.
func (h *Handler) DeleteRepository(c echo.Context) error {
	if err := h.service.DeleteRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), c.Param("repositoryID")); err != nil {
		return writeGitHubError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SyncRepository imports recent commits for a project GitHub repository.
func (h *Handler) SyncRepository(c echo.Context) error {
	result, err := h.service.SyncRepository(c.Request().Context(), c.Param("projectID"), currentUserID(c), c.Param("repositoryID"))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusAccepted, result)
}

// ListProjectCommits returns imported GitHub commits for one project.
func (h *Handler) ListProjectCommits(c echo.Context) error {
	commits, err := h.service.ListProjectCommits(c.Request().Context(), c.Param("projectID"), currentUserID(c), CommitListOptions{
		RepositoryID: c.QueryParam("repository_id"),
		Query:        c.QueryParam("q"),
		UnlinkedOnly: strings.EqualFold(c.QueryParam("unlinked"), "true"),
		Limit:        queryInt(c, "limit"),
	})
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusOK, commits)
}

// ListTicketCommits returns imported GitHub commits linked to one ticket.
func (h *Handler) ListTicketCommits(c echo.Context) error {
	commits, err := h.service.ListTicketCommits(c.Request().Context(), c.Param("ticketID"), currentUserID(c))
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusOK, commits)
}

// LinkCommitToTicket manually attaches an imported commit to a ticket.
func (h *Handler) LinkCommitToTicket(c echo.Context) error {
	var req linkCommitRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	commit, err := h.service.LinkCommitToTicket(c.Request().Context(), c.Param("ticketID"), currentUserID(c), req.CommitID)
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusCreated, commit)
}

// UnlinkCommitFromTicket removes a commit link from a ticket.
func (h *Handler) UnlinkCommitFromTicket(c echo.Context) error {
	if err := h.service.UnlinkCommitFromTicket(c.Request().Context(), c.Param("ticketID"), currentUserID(c), c.Param("commitID")); err != nil {
		return writeGitHubError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleWebhook accepts verified GitHub webhook deliveries.
func (h *Handler) HandleWebhook(c echo.Context) error {
	if h.webhookSecret == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "github webhook is not configured"})
	}
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid webhook body"})
	}
	if !validGitHubSignature(h.webhookSecret, c.Request().Header.Get("X-Hub-Signature-256"), body) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid github webhook signature"})
	}
	result, err := h.service.ProcessWebhook(
		c.Request().Context(),
		c.Request().Header.Get("X-GitHub-Event"),
		c.Request().Header.Get("X-GitHub-Delivery"),
		body,
	)
	if err != nil {
		return writeGitHubError(c, err)
	}
	return c.JSON(http.StatusAccepted, result)
}

// currentUserID returns the authenticated user ID stored by JWT middleware.
func currentUserID(c echo.Context) string {
	userID, _ := c.Get("userID").(string)
	return userID
}

// queryInt parses a positive integer query parameter.
func queryInt(c echo.Context, name string) int {
	value := strings.TrimSpace(c.QueryParam(name))
	if value == "" {
		return 0
	}
	result := 0
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0
		}
		result = result*10 + int(char-'0')
	}
	return result
}

// validGitHubSignature verifies a GitHub sha256 webhook signature.
func validGitHubSignature(secret, signature string, body []byte) bool {
	const prefix = "sha256="
	signature = strings.TrimSpace(signature)
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := prefix + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// writeGitHubError maps service errors to HTTP responses.
func writeGitHubError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrGitHubNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrGitHubRequestFailed):
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "not found or access denied"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		log.Printf("github integration request failed: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "github integration request failed"})
	}
}

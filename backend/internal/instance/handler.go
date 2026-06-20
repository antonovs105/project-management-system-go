// Package instance exposes safe runtime capabilities to clients.
package instance

import (
	"context"
	"net/http"

	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/labstack/echo/v4"
)

// RoleProvider resolves the current user's instance role.
type RoleProvider interface {
	InstanceRole(ctx context.Context, userID string) (string, error)
}

// OAuthProviderLister exposes configured OAuth provider keys.
type OAuthProviderLister interface {
	EnabledOAuthProviders() []string
}

// Handler exposes public and authenticated instance metadata.
type Handler struct {
	cfg     appconfig.Config
	roles   RoleProvider
	oauth   OAuthProviderLister
	version string
}

// PublicResponse contains safe instance metadata available before sign-in.
type PublicResponse struct {
	Name                  string   `json:"name"`
	Version               string   `json:"version"`
	RegistrationEnabled   bool     `json:"registration_enabled"`
	ProjectCreationPolicy string   `json:"project_creation_policy"`
	OAuthProviders        []string `json:"oauth_providers"`
}

// CapabilitiesResponse contains safe metadata plus current-user capabilities.
type CapabilitiesResponse struct {
	PublicResponse
	InstanceRole      string `json:"instance_role"`
	CanCreateProjects bool   `json:"can_create_projects"`
}

// NewHandler creates an instance metadata handler.
func NewHandler(cfg appconfig.Config, roles RoleProvider, oauth OAuthProviderLister) *Handler {
	return &Handler{
		cfg:     cfg,
		roles:   roles,
		oauth:   oauth,
		version: "dev",
	}
}

// RegisterPublicRoutes registers unauthenticated instance metadata routes.
func (h *Handler) RegisterPublicRoutes(e *echo.Echo) {
	e.GET("/instance", h.Public)
}

// RegisterAuthenticatedRoutes registers current-user capability routes.
func (h *Handler) RegisterAuthenticatedRoutes(api *echo.Group) {
	api.GET("/instance", h.Capabilities)
}

// Public returns safe metadata needed before authentication.
func (h *Handler) Public(c echo.Context) error {
	return c.JSON(http.StatusOK, h.publicResponse())
}

// Capabilities returns metadata and current-user instance capabilities.
func (h *Handler) Capabilities(c echo.Context) error {
	userID, _ := c.Get("userID").(string)
	role, err := h.roles.InstanceRole(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "instance role unavailable"})
	}
	response := CapabilitiesResponse{
		PublicResponse:    h.publicResponse(),
		InstanceRole:      role,
		CanCreateProjects: CanCreateProjects(h.cfg.Projects.CreationPolicy, role),
	}
	return c.JSON(http.StatusOK, response)
}

// CanCreateProjects reports whether role can create projects under policy.
func CanCreateProjects(policy string, role string) bool {
	switch policy {
	case "", appconfig.ProjectCreationEveryone:
		return true
	case appconfig.ProjectCreationAdminsOnly:
		return user.HasAdminPrivileges(role)
	default:
		return false
	}
}

// publicResponse builds the shared safe metadata response.
func (h *Handler) publicResponse() PublicResponse {
	return PublicResponse{
		Name:                  h.cfg.Instance.Name,
		Version:               h.version,
		RegistrationEnabled:   h.cfg.Registration.Enabled,
		ProjectCreationPolicy: h.cfg.Projects.CreationPolicy,
		OAuthProviders:        h.oauth.EnabledOAuthProviders(),
	}
}

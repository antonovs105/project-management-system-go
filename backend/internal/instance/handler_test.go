package instance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestPublicReturnsSafeInstanceMetadata(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Instance.Name = "Alpha"
	cfg.Registration.Enabled = false
	cfg.Projects.CreationPolicy = appconfig.ProjectCreationAdminsOnly
	handler := NewHandler(cfg, fakeRoles{}, fakeOAuth{providers: []string{user.OAuthProviderGitHub}})
	e := echo.New()
	handler.RegisterPublicRoutes(e)

	req := httptest.NewRequest(http.MethodGet, "/instance", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body PublicResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "Alpha", body.Name)
	require.Equal(t, "dev", body.Version)
	require.False(t, body.RegistrationEnabled)
	require.Equal(t, appconfig.ProjectCreationAdminsOnly, body.ProjectCreationPolicy)
	require.Equal(t, []string{user.OAuthProviderGitHub}, body.OAuthProviders)
}

func TestCapabilitiesReturnsCurrentUserPolicy(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Projects.CreationPolicy = appconfig.ProjectCreationAdminsOnly
	handler := NewHandler(cfg, fakeRoles{roles: map[string]string{"user-1": user.InstanceRoleUser}}, fakeOAuth{})
	e := echo.New()
	e.GET("/api/v1/instance", func(c echo.Context) error {
		c.Set("userID", "user-1")
		return handler.Capabilities(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body CapabilitiesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, user.InstanceRoleUser, body.InstanceRole)
	require.False(t, body.CanCreateProjects)
}

func TestCanCreateProjects(t *testing.T) {
	require.True(t, CanCreateProjects(appconfig.ProjectCreationEveryone, user.InstanceRoleUser))
	require.True(t, CanCreateProjects(appconfig.ProjectCreationAdminsOnly, user.InstanceRoleAdmin))
	require.True(t, CanCreateProjects(appconfig.ProjectCreationAdminsOnly, user.InstanceRoleOwner))
	require.False(t, CanCreateProjects(appconfig.ProjectCreationAdminsOnly, user.InstanceRoleUser))
	require.False(t, CanCreateProjects("unknown", user.InstanceRoleOwner))
}

type fakeRoles struct {
	roles map[string]string
}

func (f fakeRoles) InstanceRole(ctx context.Context, userID string) (string, error) {
	return f.roles[userID], nil
}

type fakeOAuth struct {
	providers []string
}

func (f fakeOAuth) EnabledOAuthProviders() []string {
	return f.providers
}

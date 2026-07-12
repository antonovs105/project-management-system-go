package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadFileAppliesYAMLAndEnvOverrides(t *testing.T) {
	path := writeConfig(t, `
app:
  env: " Development "
  role: " API "
server:
  http_addr: ":9000"
  cors_allowed_origins:
    - "http://localhost:5173"
database:
  source: "postgres://file"
security:
  jwt_secret_key: "file-jwt-secret"
instance:
  name: " Custom Progo "
  public_base_url: "http://localhost:8080"
  local_domain: "localhost:8080"
registration:
  enabled: false
projects:
  creation_policy: "admins_only"
rate_limits:
  auth:
    requests_per_second: 4
    burst: 8
`)
	t.Setenv("DB_SOURCE", "postgres://env")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("PROJECT_CREATION_POLICY", ProjectCreationEveryone)
	t.Setenv("AUTH_RATE_LIMIT_BURST", "12")
	t.Setenv("DB_MAX_OPEN_CONNECTIONS", "40")

	cfg, err := LoadFile(path)

	require.NoError(t, err)
	require.Equal(t, EnvDevelopment, cfg.App.Env)
	require.Equal(t, "api", cfg.App.Role)
	require.Equal(t, "Custom Progo", cfg.Instance.Name)
	require.True(t, cfg.Registration.Enabled)
	require.Equal(t, "postgres://env", cfg.Database.Source)
	require.Equal(t, ProjectCreationEveryone, cfg.Projects.CreationPolicy)
	require.Equal(t, 4.0, cfg.RateLimits.Auth.RequestsPerSecond)
	require.Equal(t, 12, cfg.RateLimits.Auth.Burst)
	require.Equal(t, 40, cfg.Database.MaxOpenConnections)
	require.Equal(t, 10, cfg.Database.MaxIdleConnections)
	require.True(t, cfg.FederationAllowInsecureHTTP())
}

func TestValidateRejectsInvalidDatabasePool(t *testing.T) {
	cfg := validConfig()
	cfg.Database.MaxOpenConnections = 5
	cfg.Database.MaxIdleConnections = 6

	err := cfg.Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "database.max_idle_connections")
}

func TestLoadFileNoEnvIgnoresEnvironmentOverrides(t *testing.T) {
	path := writeConfig(t, `
database:
  source: "postgres://file"
security:
  jwt_secret_key: "file-jwt-secret"
instance:
  public_base_url: "http://localhost:8080"
  local_domain: "localhost:8080"
registration:
  enabled: false
projects:
  creation_policy: "admins_only"
`)
	t.Setenv("DB_SOURCE", "postgres://env")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("PROJECT_CREATION_POLICY", ProjectCreationEveryone)

	cfg, err := LoadFileNoEnv(path)

	require.NoError(t, err)
	require.Equal(t, "postgres://file", cfg.Database.Source)
	require.False(t, cfg.Registration.Enabled)
	require.Equal(t, ProjectCreationAdminsOnly, cfg.Projects.CreationPolicy)
}

func TestLoadFileRejectsUnknownYAMLFields(t *testing.T) {
	path := writeConfig(t, `
database:
  source: "postgres://file"
wat: true
`)

	_, err := LoadFile(path)

	require.Error(t, err)
	require.Contains(t, err.Error(), "field wat not found")
}

func TestValidateRejectsInvalidProjectCreationPolicy(t *testing.T) {
	cfg := validConfig()
	cfg.Projects.CreationPolicy = "managers"

	err := cfg.Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "projects.creation_policy")
}

func TestValidateRejectsUnsafeProductionConfig(t *testing.T) {
	cfg := validConfig()
	cfg.App.Env = EnvProduction
	cfg.Instance.PublicBaseURL = "http://progo.example.test"
	cfg.Security.JWTSecretKey = strings.Repeat("j", 32)
	cfg.Server.CORSAllowedOrigins = []string{"https://progo.example.test"}
	cfg.Metrics.Token = strings.Repeat("m", 32)
	cfg.Security.ActorPrivateKeyEncryptionKey = strings.Repeat("a", 32)

	err := cfg.Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "https")
}

func TestValidateAcceptsProductionConfig(t *testing.T) {
	disabled := false
	cfg := validConfig()
	cfg.App.Env = EnvProduction
	cfg.Security.JWTSecretKey = strings.Repeat("j", 32)
	cfg.Security.ActorPrivateKeyEncryptionKey = strings.Repeat("a", 32)
	cfg.Instance.PublicBaseURL = "https://progo.example.test"
	cfg.Instance.LocalDomain = "progo.example.test"
	cfg.Server.CORSAllowedOrigins = []string{"https://progo.example.test"}
	cfg.Metrics.Token = strings.Repeat("m", 32)
	cfg.Federation.AllowInsecureHTTP = &disabled
	cfg.Email.Host = "smtp.example.test"
	cfg.Email.FromAddress = "progo@example.test"

	require.NoError(t, cfg.Validate())
}

func TestValidateRequiresProductionCORSForAPIRoles(t *testing.T) {
	cfg := validProductionConfig()
	cfg.App.Role = RoleAPI
	cfg.Server.CORSAllowedOrigins = nil

	err := cfg.Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "server.cors_allowed_origins")
}

func TestValidateDoesNotRequireProductionCORSForWorkerRole(t *testing.T) {
	cfg := validProductionConfig()
	cfg.App.Role = RoleWorker
	cfg.Server.CORSAllowedOrigins = nil

	require.NoError(t, cfg.Validate())
}

func TestFederationAllowInsecureHTTPCanBeExplicitlyDisabledInDevelopment(t *testing.T) {
	disabled := false
	cfg := validConfig()
	cfg.Federation.AllowInsecureHTTP = &disabled

	require.False(t, cfg.FederationAllowInsecureHTTP())
}

func validConfig() Config {
	cfg := Default()
	cfg.Database.Source = "postgres://postgres:postgres@localhost:5432/pms?sslmode=disable"
	cfg.Security.JWTSecretKey = "dev-secret"
	cfg.Instance.PublicBaseURL = "http://localhost:8080"
	cfg.Instance.LocalDomain = "localhost:8080"
	return cfg
}

func validProductionConfig() Config {
	disabled := false
	cfg := validConfig()
	cfg.App.Env = EnvProduction
	cfg.Security.JWTSecretKey = strings.Repeat("j", 32)
	cfg.Security.ActorPrivateKeyEncryptionKey = strings.Repeat("a", 32)
	cfg.Instance.PublicBaseURL = "https://progo.example.test"
	cfg.Instance.LocalDomain = "progo.example.test"
	cfg.Server.CORSAllowedOrigins = []string{"https://progo.example.test"}
	cfg.Metrics.Token = strings.Repeat("m", 32)
	cfg.Federation.AllowInsecureHTTP = &disabled
	cfg.Email.Host = "smtp.example.test"
	cfg.Email.FromAddress = "progo@example.test"
	return cfg
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "progo.yml")
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o600))
	return path
}

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
  public_base_url: "http://localhost:8080"
  local_domain: "localhost:8080"
projects:
  creation_policy: "admins_only"
rate_limits:
  auth:
    requests_per_second: 4
    burst: 8
`)
	t.Setenv("DB_SOURCE", "postgres://env")
	t.Setenv("PROJECT_CREATION_POLICY", ProjectCreationEveryone)
	t.Setenv("AUTH_RATE_LIMIT_BURST", "12")

	cfg, err := LoadFile(path)

	require.NoError(t, err)
	require.Equal(t, EnvDevelopment, cfg.App.Env)
	require.Equal(t, "api", cfg.App.Role)
	require.Equal(t, "postgres://env", cfg.Database.Source)
	require.Equal(t, ProjectCreationEveryone, cfg.Projects.CreationPolicy)
	require.Equal(t, 4.0, cfg.RateLimits.Auth.RequestsPerSecond)
	require.Equal(t, 12, cfg.RateLimits.Auth.Burst)
	require.True(t, cfg.FederationAllowInsecureHTTP())
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

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "progo.yml")
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o600))
	return path
}

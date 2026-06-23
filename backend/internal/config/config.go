// Package config loads and validates Progo runtime configuration.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// EnvDevelopment enables local defaults and relaxed deployment checks.
	EnvDevelopment = "development"
	// EnvTest marks test processes that still use development-grade defaults.
	EnvTest = "test"
	// EnvProduction enables strict deployment safety checks.
	EnvProduction = "production"

	// RoleAPI runs only the HTTP API process.
	RoleAPI = "api"
	// RoleWorker runs only the ActivityPub delivery worker process.
	RoleWorker = "worker"
	// RoleAll runs the HTTP API and delivery worker in one process.
	RoleAll = "all"

	// ProjectCreationEveryone lets every local account create projects.
	ProjectCreationEveryone = "everyone"
	// ProjectCreationAdminsOnly limits new projects to instance owners/admins.
	ProjectCreationAdminsOnly = "admins_only"

	// defaultConfigPath is loaded automatically when present in the working directory.
	defaultConfigPath = "progo.yml"
)

// Config is the typed runtime configuration used by API and maintenance tools.
type Config struct {
	App          AppConfig          `yaml:"app"`
	Server       ServerConfig       `yaml:"server"`
	Database     DatabaseConfig     `yaml:"database"`
	Security     SecurityConfig     `yaml:"security"`
	Instance     InstanceConfig     `yaml:"instance"`
	Registration RegistrationConfig `yaml:"registration"`
	Projects     ProjectsConfig     `yaml:"projects"`
	RateLimits   RateLimitsConfig   `yaml:"rate_limits"`
	Redis        RedisConfig        `yaml:"redis"`
	Metrics      MetricsConfig      `yaml:"metrics"`
	Federation   FederationConfig   `yaml:"federation"`
	OAuth        OAuthConfig        `yaml:"oauth"`
	GitHub       GitHubConfig       `yaml:"github"`
}

// AppConfig controls process mode.
type AppConfig struct {
	Env  string `yaml:"env"`
	Role string `yaml:"role"`
}

// ServerConfig controls HTTP-facing settings.
type ServerConfig struct {
	HTTPAddr           string   `yaml:"http_addr"`
	RequestBodyLimit   string   `yaml:"request_body_limit"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	TrustedProxyCIDRs  []string `yaml:"trusted_proxy_cidrs"`
}

// DatabaseConfig controls PostgreSQL connectivity.
type DatabaseConfig struct {
	Source string `yaml:"source"`
}

// SecurityConfig contains local application secrets.
type SecurityConfig struct {
	JWTSecretKey                 string `yaml:"jwt_secret_key"`
	ActorPrivateKeyEncryptionKey string `yaml:"actor_private_key_encryption_key"`
}

// InstanceConfig describes the local federated instance identity.
type InstanceConfig struct {
	Name          string `yaml:"name"`
	PublicBaseURL string `yaml:"public_base_url"`
	LocalDomain   string `yaml:"local_domain"`
}

// RegistrationConfig controls public account creation.
type RegistrationConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ProjectsConfig controls instance-wide project policies.
type ProjectsConfig struct {
	CreationPolicy string `yaml:"creation_policy"`
}

// RateLimitsConfig groups public endpoint throttles.
type RateLimitsConfig struct {
	Auth      RateLimitConfig `yaml:"auth"`
	Discovery RateLimitConfig `yaml:"discovery"`
	Inbox     RateLimitConfig `yaml:"inbox"`
}

// RateLimitConfig controls one in-memory rate limiter.
type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

// RedisConfig controls background queue connectivity.
type RedisConfig struct {
	Addr string `yaml:"addr"`
}

// MetricsConfig controls the protected metrics listener.
type MetricsConfig struct {
	Addr  string `yaml:"addr"`
	Token string `yaml:"token"`
}

// FederationConfig controls ActivityPub safety settings.
type FederationConfig struct {
	BlockedDomains       []string `yaml:"blocked_domains"`
	AllowInsecureHTTP    *bool    `yaml:"allow_insecure_http"`
	AllowPrivateNetworks *bool    `yaml:"allow_private_networks"`
}

// OAuthConfig controls optional third-party login providers.
type OAuthConfig struct {
	FrontendCallbackURL string              `yaml:"frontend_callback_url"`
	Google              OAuthProviderConfig `yaml:"google"`
	GitHub              OAuthProviderConfig `yaml:"github"`
}

// OAuthProviderConfig contains one OAuth provider's client settings.
type OAuthProviderConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

// GitHubConfig controls optional GitHub repository integration.
type GitHubConfig struct {
	APIToken      string `yaml:"api_token"`
	WebhookSecret string `yaml:"webhook_secret"`
}

// Default returns development-safe defaults before file/env overrides.
func Default() Config {
	return Config{
		App: AppConfig{
			Env:  EnvDevelopment,
			Role: RoleAll,
		},
		Server: ServerConfig{
			HTTPAddr:         ":8080",
			RequestBodyLimit: "2M",
		},
		Instance: InstanceConfig{
			Name: "Progo",
		},
		Registration: RegistrationConfig{
			Enabled: true,
		},
		Projects: ProjectsConfig{
			CreationPolicy: ProjectCreationEveryone,
		},
		RateLimits: RateLimitsConfig{
			Auth:      RateLimitConfig{RequestsPerSecond: 2, Burst: 10},
			Discovery: RateLimitConfig{RequestsPerSecond: 10, Burst: 50},
			Inbox:     RateLimitConfig{RequestsPerSecond: 20, Burst: 100},
		},
		OAuth: OAuthConfig{
			FrontendCallbackURL: "http://localhost:5173/oauth/callback",
		},
	}
}

// Load reads the configured YAML file and applies environment overrides.
func Load() (Config, error) {
	return LoadFile(strings.TrimSpace(os.Getenv("PROGO_CONFIG")))
}

// LoadFile reads a specific YAML file and applies environment overrides.
func LoadFile(path string) (Config, error) {
	return loadFile(path, true)
}

// LoadFileNoEnv reads a specific YAML file without applying environment overrides.
func LoadFileNoEnv(path string) (Config, error) {
	return loadFile(path, false)
}

// loadFile reads YAML configuration and optionally applies environment overrides.
func loadFile(path string, useEnv bool) (Config, error) {
	cfg := Default()
	path = strings.TrimSpace(path)
	if path == "" {
		if _, err := os.Stat(defaultConfigPath); err == nil {
			path = defaultConfigPath
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return Config{}, err
		}
		defer file.Close()

		decoder := yaml.NewDecoder(file)
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("load config %s: %w", path, err)
		}
	}

	if useEnv {
		if err := applyEnv(&cfg); err != nil {
			return Config{}, err
		}
	}
	Normalize(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Normalize canonicalizes operator-facing string tokens before validation.
func Normalize(cfg *Config) {
	cfg.App.Env = strings.ToLower(strings.TrimSpace(cfg.App.Env))
	cfg.App.Role = strings.ToLower(strings.TrimSpace(cfg.App.Role))
	cfg.Projects.CreationPolicy = strings.ToLower(strings.TrimSpace(cfg.Projects.CreationPolicy))
	cfg.Server.HTTPAddr = strings.TrimSpace(cfg.Server.HTTPAddr)
	cfg.Server.RequestBodyLimit = strings.TrimSpace(cfg.Server.RequestBodyLimit)
	cfg.Database.Source = strings.TrimSpace(cfg.Database.Source)
	cfg.Security.JWTSecretKey = strings.TrimSpace(cfg.Security.JWTSecretKey)
	cfg.Security.ActorPrivateKeyEncryptionKey = strings.TrimSpace(cfg.Security.ActorPrivateKeyEncryptionKey)
	cfg.Instance.Name = strings.TrimSpace(cfg.Instance.Name)
	if cfg.Instance.Name == "" {
		cfg.Instance.Name = "Progo"
	}
	cfg.Instance.PublicBaseURL = strings.TrimSpace(cfg.Instance.PublicBaseURL)
	cfg.Instance.LocalDomain = strings.TrimSpace(cfg.Instance.LocalDomain)
	cfg.Redis.Addr = strings.TrimSpace(cfg.Redis.Addr)
	cfg.Metrics.Addr = strings.TrimSpace(cfg.Metrics.Addr)
	cfg.Metrics.Token = strings.TrimSpace(cfg.Metrics.Token)
	cfg.OAuth.FrontendCallbackURL = strings.TrimSpace(cfg.OAuth.FrontendCallbackURL)
	cfg.OAuth.Google.ClientID = strings.TrimSpace(cfg.OAuth.Google.ClientID)
	cfg.OAuth.Google.ClientSecret = strings.TrimSpace(cfg.OAuth.Google.ClientSecret)
	cfg.OAuth.Google.RedirectURL = strings.TrimSpace(cfg.OAuth.Google.RedirectURL)
	cfg.OAuth.GitHub.ClientID = strings.TrimSpace(cfg.OAuth.GitHub.ClientID)
	cfg.OAuth.GitHub.ClientSecret = strings.TrimSpace(cfg.OAuth.GitHub.ClientSecret)
	cfg.OAuth.GitHub.RedirectURL = strings.TrimSpace(cfg.OAuth.GitHub.RedirectURL)
	cfg.GitHub.APIToken = strings.TrimSpace(cfg.GitHub.APIToken)
	cfg.GitHub.WebhookSecret = strings.TrimSpace(cfg.GitHub.WebhookSecret)
	cfg.Server.CORSAllowedOrigins = trimList(cfg.Server.CORSAllowedOrigins)
	cfg.Server.TrustedProxyCIDRs = trimList(cfg.Server.TrustedProxyCIDRs)
	cfg.Federation.BlockedDomains = trimList(cfg.Federation.BlockedDomains)
}

// Validate checks that the configuration is coherent for the selected app env.
func (c Config) Validate() error {
	if !isOneOf(c.App.Env, EnvDevelopment, EnvTest, EnvProduction) {
		return fmt.Errorf("app.env must be one of %s, %s, or %s", EnvDevelopment, EnvTest, EnvProduction)
	}
	if !isOneOf(c.App.Role, RoleAPI, RoleWorker, RoleAll) {
		return fmt.Errorf("app.role must be one of %s, %s, or %s", RoleAPI, RoleWorker, RoleAll)
	}
	if strings.TrimSpace(c.Database.Source) == "" {
		return fmt.Errorf("database.source is required")
	}
	if strings.TrimSpace(c.Security.JWTSecretKey) == "" {
		return fmt.Errorf("security.jwt_secret_key is required")
	}
	if strings.TrimSpace(c.Instance.PublicBaseURL) == "" {
		return fmt.Errorf("instance.public_base_url is required")
	}
	if strings.TrimSpace(c.Instance.LocalDomain) == "" {
		return fmt.Errorf("instance.local_domain is required")
	}
	if !isOneOf(c.Projects.CreationPolicy, ProjectCreationEveryone, ProjectCreationAdminsOnly) {
		return fmt.Errorf("projects.creation_policy must be one of %s or %s", ProjectCreationEveryone, ProjectCreationAdminsOnly)
	}
	if err := validateRateLimit("rate_limits.auth", c.RateLimits.Auth); err != nil {
		return err
	}
	if err := validateRateLimit("rate_limits.discovery", c.RateLimits.Discovery); err != nil {
		return err
	}
	if err := validateRateLimit("rate_limits.inbox", c.RateLimits.Inbox); err != nil {
		return err
	}
	parsedBaseURL, err := url.Parse(strings.TrimSpace(c.Instance.PublicBaseURL))
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return fmt.Errorf("instance.public_base_url must be an absolute HTTP URL")
	}
	if parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https" {
		return fmt.Errorf("instance.public_base_url must use http or https")
	}
	if strings.ContainsAny(c.Instance.LocalDomain, " \t\r\n/") {
		return fmt.Errorf("instance.local_domain must be a host name, not a URL")
	}
	if err := validateCORS(c.App.Env == EnvProduction && roleServesHTTP(c.App.Role), c.Server.CORSAllowedOrigins); err != nil {
		return err
	}

	if c.App.Env == EnvProduction {
		if len(c.Security.JWTSecretKey) < 32 || c.Security.JWTSecretKey == "your_secret_key_here" {
			return fmt.Errorf("security.jwt_secret_key must be a production-grade secret")
		}
		if parsedBaseURL.Scheme != "https" {
			return fmt.Errorf("instance.public_base_url must use https in production")
		}
		if isLocalHost(parsedBaseURL.Hostname()) || isLocalHost(c.Instance.LocalDomain) {
			return fmt.Errorf("instance.public_base_url and instance.local_domain must not use localhost in production")
		}
		if len(strings.TrimSpace(c.Metrics.Token)) < 32 {
			return fmt.Errorf("metrics.token must be at least 32 characters in production")
		}
		if len(strings.TrimSpace(c.Security.ActorPrivateKeyEncryptionKey)) < 32 {
			return fmt.Errorf("security.actor_private_key_encryption_key must be at least 32 characters in production")
		}
		if c.FederationAllowInsecureHTTP() {
			return fmt.Errorf("federation.allow_insecure_http cannot be enabled in production")
		}
		if c.FederationAllowPrivateNetworks() {
			return fmt.Errorf("federation.allow_private_networks cannot be enabled in production")
		}
	}

	return nil
}

// roleServesHTTP reports whether an app role exposes browser-facing HTTP routes.
func roleServesHTTP(role string) bool {
	return role == RoleAPI || role == RoleAll
}

// FederationAllowInsecureHTTP applies the development default when unset.
func (c Config) FederationAllowInsecureHTTP() bool {
	if c.Federation.AllowInsecureHTTP != nil {
		return *c.Federation.AllowInsecureHTTP
	}
	return c.App.Env != EnvProduction
}

// FederationAllowPrivateNetworks applies the production-safe default when unset.
func (c Config) FederationAllowPrivateNetworks() bool {
	return c.Federation.AllowPrivateNetworks != nil && *c.Federation.AllowPrivateNetworks
}

// applyEnv overlays legacy environment variables on top of YAML config values.
func applyEnv(cfg *Config) error {
	applyStringEnv(&cfg.App.Env, "APP_ENV")
	applyStringEnv(&cfg.App.Role, "APP_ROLE")
	applyStringEnv(&cfg.Database.Source, "DB_SOURCE")
	applyStringEnv(&cfg.Security.JWTSecretKey, "JWT_SECRET_KEY")
	applyStringEnv(&cfg.Instance.PublicBaseURL, "PUBLIC_BASE_URL")
	applyStringEnv(&cfg.Instance.LocalDomain, "LOCAL_DOMAIN")
	applyStringEnv(&cfg.Instance.Name, "INSTANCE_NAME")
	applyStringEnv(&cfg.Redis.Addr, "REDIS_ADDR")
	applyStringEnv(&cfg.Metrics.Addr, "METRICS_ADDR")
	applyStringEnv(&cfg.Metrics.Token, "METRICS_TOKEN")
	applyStringEnv(&cfg.Security.ActorPrivateKeyEncryptionKey, "ACTOR_PRIVATE_KEY_ENCRYPTION_KEY")
	applyStringEnv(&cfg.OAuth.FrontendCallbackURL, "OAUTH_FRONTEND_CALLBACK_URL")
	applyStringEnv(&cfg.OAuth.Google.ClientID, "GOOGLE_OAUTH_CLIENT_ID")
	applyStringEnv(&cfg.OAuth.Google.ClientSecret, "GOOGLE_OAUTH_CLIENT_SECRET")
	applyStringEnv(&cfg.OAuth.Google.RedirectURL, "GOOGLE_OAUTH_REDIRECT_URL")
	applyStringEnv(&cfg.OAuth.GitHub.ClientID, "GITHUB_OAUTH_CLIENT_ID")
	applyStringEnv(&cfg.OAuth.GitHub.ClientSecret, "GITHUB_OAUTH_CLIENT_SECRET")
	applyStringEnv(&cfg.OAuth.GitHub.RedirectURL, "GITHUB_OAUTH_REDIRECT_URL")
	applyStringEnv(&cfg.GitHub.APIToken, "GITHUB_API_TOKEN")
	applyStringEnv(&cfg.GitHub.WebhookSecret, "GITHUB_WEBHOOK_SECRET")
	applyStringEnv(&cfg.Projects.CreationPolicy, "PROJECT_CREATION_POLICY")
	if err := applyBoolValueEnv(&cfg.Registration.Enabled, "REGISTRATION_ENABLED"); err != nil {
		return err
	}
	applyCSVEnv(&cfg.Federation.BlockedDomains, "FEDERATION_BLOCKED_DOMAINS")
	applyCSVEnv(&cfg.Server.CORSAllowedOrigins, "CORS_ALLOWED_ORIGINS")
	applyCSVEnv(&cfg.Server.TrustedProxyCIDRs, "TRUSTED_PROXY_CIDRS")
	if err := applyBoolEnv(&cfg.Federation.AllowInsecureHTTP, "FEDERATION_ALLOW_INSECURE_HTTP"); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.Federation.AllowPrivateNetworks, "FEDERATION_ALLOW_PRIVATE_NETWORKS"); err != nil {
		return err
	}
	if err := applyFloatEnv(&cfg.RateLimits.Auth.RequestsPerSecond, "AUTH_RATE_LIMIT_PER_SECOND"); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.RateLimits.Auth.Burst, "AUTH_RATE_LIMIT_BURST"); err != nil {
		return err
	}
	if err := applyFloatEnv(&cfg.RateLimits.Discovery.RequestsPerSecond, "DISCOVERY_RATE_LIMIT_PER_SECOND"); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.RateLimits.Discovery.Burst, "DISCOVERY_RATE_LIMIT_BURST"); err != nil {
		return err
	}
	if err := applyFloatEnv(&cfg.RateLimits.Inbox.RequestsPerSecond, "INBOX_RATE_LIMIT_PER_SECOND"); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.RateLimits.Inbox.Burst, "INBOX_RATE_LIMIT_BURST"); err != nil {
		return err
	}
	return nil
}

// applyStringEnv replaces target when name is set to a non-empty value.
func applyStringEnv(target *string, name string) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		*target = value
	}
}

// applyCSVEnv replaces target with a trimmed comma-separated environment value.
func applyCSVEnv(target *[]string, name string) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		*target = splitCSV(value)
	}
}

// applyBoolEnv replaces target with an explicitly configured boolean value.
func applyBoolEnv(target **bool, name string) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	value, err := parseBool(raw)
	if err != nil {
		return fmt.Errorf("%s must be a boolean", name)
	}
	*target = &value
	return nil
}

// applyBoolValueEnv replaces target with an explicitly configured boolean value.
func applyBoolValueEnv(target *bool, name string) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	value, err := parseBool(raw)
	if err != nil {
		return fmt.Errorf("%s must be a boolean", name)
	}
	*target = value
	return nil
}

// applyFloatEnv replaces target with a parsed floating-point environment value.
func applyFloatEnv(target *float64, name string) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("%s must be a number", name)
	}
	*target = value
	return nil
}

// applyIntEnv replaces target with a parsed integer environment value.
func applyIntEnv(target *int, name string) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%s must be an integer", name)
	}
	*target = value
	return nil
}

// parseBool accepts common operator-friendly boolean strings.
func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}

// validateRateLimit checks one named rate limiter configuration.
func validateRateLimit(label string, cfg RateLimitConfig) error {
	if cfg.RequestsPerSecond <= 0 {
		return fmt.Errorf("%s.requests_per_second must be greater than zero", label)
	}
	if cfg.Burst <= 0 {
		return fmt.Errorf("%s.burst must be greater than zero", label)
	}
	return nil
}

// validateCORS rejects unsafe browser origins when production checks are active.
func validateCORS(production bool, origins []string) error {
	if !production {
		return nil
	}
	if len(origins) == 0 {
		return fmt.Errorf("server.cors_allowed_origins is required in production")
	}
	for _, origin := range origins {
		if origin == "*" {
			return fmt.Errorf("server.cors_allowed_origins must not include wildcard origins in production")
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" {
			return fmt.Errorf("server.cors_allowed_origins must contain absolute HTTP origins")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("server.cors_allowed_origins must use http or https")
		}
		if isLocalHost(parsed.Hostname()) {
			return fmt.Errorf("server.cors_allowed_origins must not use localhost in production")
		}
	}
	return nil
}

// isOneOf reports whether value exactly matches one allowed token.
func isOneOf(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// isLocalHost reports whether host points at loopback development addresses.
func isLocalHost(host string) bool {
	host = strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// splitCSV parses comma-separated config values while dropping empty entries.
func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

// trimList removes empty entries after trimming operator-provided lists.
func trimList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

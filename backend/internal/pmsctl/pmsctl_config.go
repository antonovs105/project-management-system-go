package pmsctl

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"gopkg.in/yaml.v3"
)

// configInitOptions carries validated config init flags.
type configInitOptions struct {
	Output string
	Force  bool
}

// configExportEnvOptions carries validated config export flags.
type configExportEnvOptions struct {
	ConfigFile  string
	Output      string
	Force       bool
	DeployName  string
	ImagePrefix string
	ImageTag    string
}

// dotenvEntry is one key/value line in generated deployment env files.
type dotenvEntry struct {
	Key   string
	Value string
}

// dotenvRawValuePattern matches values that do not need dotenv quoting.
var dotenvRawValuePattern = regexp.MustCompile(`^[A-Za-z0-9_./:@,+%=-]*$`)

// runConfigExportEnv creates a Docker Compose env file from runtime YAML.
func (r *Runner) runConfigExportEnv(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("pmsctl config export-env", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	configFile := fs.String("config", "progo.yml", "YAML config file to export")
	output := fs.String("output", ".env", "dotenv file to write")
	force := fs.Bool("force", false, "overwrite an existing dotenv file")
	deployName := fs.String("deploy-name", "", "container-safe instance name for Compose resources")
	imagePrefix := fs.String("image-prefix", "", "optional container image prefix")
	imageTag := fs.String("image-tag", "", "optional container image tag")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl config export-env [--config FILE] [--output FILE] [--force]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(r.Stderr, "unexpected arguments: %s\n\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return 2
	}
	options := configExportEnvOptions{
		ConfigFile:  strings.TrimSpace(*configFile),
		Output:      strings.TrimSpace(*output),
		Force:       *force,
		DeployName:  strings.TrimSpace(*deployName),
		ImagePrefix: strings.TrimSpace(*imagePrefix),
		ImageTag:    strings.TrimSpace(*imageTag),
	}
	if options.ConfigFile == "" {
		fmt.Fprintln(r.Stderr, "--config is required")
		return 2
	}
	if options.Output == "" {
		fmt.Fprintln(r.Stderr, "--output is required")
		return 2
	}
	exists, err := r.FileExists(options.Output)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to inspect output file: %v\n", err)
		return 1
	}
	if exists && !options.Force {
		fmt.Fprintf(r.Stderr, "dotenv file %q already exists; pass --force to overwrite\n", options.Output)
		return 1
	}

	cfg, err := appconfig.LoadFileNoEnv(options.ConfigFile)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to load config: %v\n", err)
		return 1
	}
	data, err := renderComposeEnv(cfg, options)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to render dotenv: %v\n", err)
		return 1
	}
	if err := r.WriteFile(options.Output, data, 0o600); err != nil {
		fmt.Fprintf(r.Stderr, "failed to write dotenv: %v\n", err)
		return 1
	}
	fmt.Fprintf(r.Stdout, "dotenv_exported path=%s config=%s instance=%s\n", options.Output, options.ConfigFile, composeDeployName(cfg, options.DeployName))
	return 0
}

// runConfigInit interactively creates a runtime YAML configuration.
func (r *Runner) runConfigInit(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("pmsctl config init", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	output := fs.String("output", "progo.yml", "YAML config file to write")
	force := fs.Bool("force", false, "overwrite an existing config file")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl config init [--output FILE] [--force]")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(r.Stderr, "unexpected arguments: %s\n\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return 2
	}
	options := configInitOptions{Output: strings.TrimSpace(*output), Force: *force}
	if options.Output == "" {
		fmt.Fprintln(r.Stderr, "--output is required")
		return 2
	}
	exists, err := r.FileExists(options.Output)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to inspect output file: %v\n", err)
		return 1
	}
	if exists && !options.Force {
		fmt.Fprintf(r.Stderr, "config file %q already exists; pass --force to overwrite\n", options.Output)
		return 1
	}

	cfg, err := r.promptConfigInit()
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to initialize config: %v\n", err)
		return 1
	}
	appconfig.Normalize(&cfg)
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(r.Stderr, "generated config is invalid: %v\n", err)
		return 1
	}
	data, err := renderConfigYAML(cfg)
	if err != nil {
		fmt.Fprintf(r.Stderr, "failed to render config: %v\n", err)
		return 1
	}
	if err := r.WriteFile(options.Output, data, 0o600); err != nil {
		fmt.Fprintf(r.Stderr, "failed to write config: %v\n", err)
		return 1
	}
	fmt.Fprintf(
		r.Stdout,
		"config_initialized path=%s app_env=%s registration_enabled=%t project_creation_policy=%s\n",
		options.Output,
		cfg.App.Env,
		cfg.Registration.Enabled,
		cfg.Projects.CreationPolicy,
	)
	return 0
}

// promptConfigInit collects operator choices and generated secrets.
func (r *Runner) promptConfigInit() (appconfig.Config, error) {
	reader := bufio.NewReader(r.Stdin)
	cfg := appconfig.Default()

	appEnv, err := promptChoice(reader, r.Stdout, "App environment", cfg.App.Env, []string{appconfig.EnvDevelopment, appconfig.EnvProduction})
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.App.Env = appEnv
	cfg.App.Role = appconfig.RoleAll

	cfg.Instance.Name, err = promptString(reader, r.Stdout, "Instance name", cfg.Instance.Name)
	if err != nil {
		return appconfig.Config{}, err
	}
	publicDefault := defaultPublicBaseURL(appEnv)
	cfg.Instance.PublicBaseURL, err = promptString(reader, r.Stdout, "Public base URL", publicDefault)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Instance.LocalDomain, err = promptString(reader, r.Stdout, "Local domain", defaultLocalDomain(cfg.Instance.PublicBaseURL, appEnv))
	if err != nil {
		return appconfig.Config{}, err
	}
	databaseDefault, err := defaultDatabaseSource(appEnv, r.GenerateSecret)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Database.Source, err = promptString(reader, r.Stdout, "Database source", databaseDefault)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Redis.Addr, err = promptString(reader, r.Stdout, "Redis address", "localhost:6379")
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Server.CORSAllowedOrigins, err = promptCSV(reader, r.Stdout, "CORS allowed origins", defaultCORSOrigins(appEnv, cfg.Instance.PublicBaseURL))
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Registration.Enabled, err = promptBool(reader, r.Stdout, "Allow public registration", cfg.Registration.Enabled)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Projects.CreationPolicy, err = promptChoice(reader, r.Stdout, "Project creation policy", cfg.Projects.CreationPolicy, []string{appconfig.ProjectCreationEveryone, appconfig.ProjectCreationAdminsOnly})
	if err != nil {
		return appconfig.Config{}, err
	}
	allowInsecureHTTP, err := promptBool(reader, r.Stdout, "Allow insecure HTTP federation", appEnv != appconfig.EnvProduction)
	if err != nil {
		return appconfig.Config{}, err
	}
	allowPrivateNetworks, err := promptBool(reader, r.Stdout, "Allow private federation networks", false)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Federation.AllowInsecureHTTP = &allowInsecureHTTP
	cfg.Federation.AllowPrivateNetworks = &allowPrivateNetworks

	cfg.Security.JWTSecretKey, err = r.GenerateSecret(32)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Security.ActorPrivateKeyEncryptionKey, err = r.GenerateSecret(32)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.Metrics.Token, err = r.GenerateSecret(32)
	if err != nil {
		return appconfig.Config{}, err
	}
	cfg.OAuth.FrontendCallbackURL = defaultFrontendCallbackURL(appEnv, cfg.Instance.PublicBaseURL)
	cfg.OAuth.Google.RedirectURL = joinPublicURL(cfg.Instance.PublicBaseURL, "/auth/google/callback")
	cfg.OAuth.GitHub.RedirectURL = joinPublicURL(cfg.Instance.PublicBaseURL, "/auth/github/callback")

	return cfg, nil
}

// promptString asks for a free-form value and applies a default on blank input.
func promptString(reader *bufio.Reader, output io.Writer, label string, defaultValue string) (string, error) {
	fmt.Fprintf(output, "%s [%s]: ", label, defaultValue)
	value, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// promptChoice asks until the user enters one of the allowed tokens.
func promptChoice(reader *bufio.Reader, output io.Writer, label string, defaultValue string, allowed []string) (string, error) {
	for {
		value, err := promptString(reader, output, fmt.Sprintf("%s (%s)", label, strings.Join(allowed, "/")), defaultValue)
		if err != nil {
			return "", err
		}
		value = strings.ToLower(strings.TrimSpace(value))
		for _, candidate := range allowed {
			if value == candidate {
				return value, nil
			}
		}
		fmt.Fprintf(output, "Please enter one of: %s\n", strings.Join(allowed, ", "))
	}
}

// promptBool asks for a yes/no value and applies a default on blank input.
func promptBool(reader *bufio.Reader, output io.Writer, label string, defaultValue bool) (bool, error) {
	defaultLabel := "n"
	if defaultValue {
		defaultLabel = "y"
	}
	for {
		value, err := promptString(reader, output, label+" (y/n)", defaultLabel)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes", "true", "1", "on":
			return true, nil
		case "n", "no", "false", "0", "off":
			return false, nil
		default:
			fmt.Fprintln(output, "Please enter yes or no.")
		}
	}
}

// promptCSV asks for a comma-separated list and trims empty entries.
func promptCSV(reader *bufio.Reader, output io.Writer, label string, defaultValues []string) ([]string, error) {
	value, err := promptString(reader, output, label, strings.Join(defaultValues, ","))
	if err != nil {
		return nil, err
	}
	return splitPromptCSV(value), nil
}

// readPromptLine reads one interactive response.
func readPromptLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && value != "") {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// splitPromptCSV parses operator-entered comma-separated values.
func splitPromptCSV(raw string) []string {
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

// renderConfigYAML serializes generated runtime config.
func renderConfigYAML(cfg appconfig.Config) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	header := "# Progo runtime configuration generated by pmsctl config init.\n"
	return append([]byte(header), data...), nil
}

// renderComposeEnv serializes runtime config for Docker Compose deployment.
func renderComposeEnv(cfg appconfig.Config, options configExportEnvOptions) ([]byte, error) {
	db, err := postgresEnv(cfg.Database.Source)
	if err != nil {
		return nil, err
	}
	deployName := composeDeployName(cfg, options.DeployName)
	if deployName == "" {
		return nil, fmt.Errorf("deploy name cannot be empty")
	}

	entries := []dotenvEntry{
		{Key: "INSTANCE_NAME", Value: deployName},
		{Key: "APP_ENV", Value: cfg.App.Env},
		{Key: "POSTGRES_USER", Value: db.User},
		{Key: "POSTGRES_PASSWORD", Value: db.Password},
		{Key: "POSTGRES_DB", Value: db.Database},
		{Key: "PUBLIC_BASE_URL", Value: cfg.Instance.PublicBaseURL},
		{Key: "LOCAL_DOMAIN", Value: cfg.Instance.LocalDomain},
		{Key: "CORS_ALLOWED_ORIGINS", Value: strings.Join(cfg.Server.CORSAllowedOrigins, ",")},
		{Key: "TRUSTED_PROXY_CIDRS", Value: strings.Join(cfg.Server.TrustedProxyCIDRs, ",")},
		{Key: "JWT_SECRET_KEY", Value: cfg.Security.JWTSecretKey},
		{Key: "METRICS_TOKEN", Value: cfg.Metrics.Token},
		{Key: "ACTOR_PRIVATE_KEY_ENCRYPTION_KEY", Value: cfg.Security.ActorPrivateKeyEncryptionKey},
		{Key: "ACTOR_PRIVATE_KEY_PREVIOUS_ENCRYPTION_KEYS", Value: strings.Join(cfg.Security.ActorPrivateKeyPreviousKeys, ",")},
		{Key: "SMTP_HOST", Value: cfg.Email.Host},
		{Key: "SMTP_PORT", Value: fmt.Sprintf("%d", cfg.Email.Port)},
		{Key: "SMTP_USERNAME", Value: cfg.Email.Username},
		{Key: "SMTP_PASSWORD", Value: cfg.Email.Password},
		{Key: "SMTP_FROM_ADDRESS", Value: cfg.Email.FromAddress},
		{Key: "SMTP_FROM_NAME", Value: cfg.Email.FromName},
		{Key: "SMTP_IMPLICIT_TLS", Value: fmt.Sprintf("%t", cfg.Email.ImplicitTLS)},
		{Key: "ATTACHMENTS_ENABLED", Value: fmt.Sprintf("%t", cfg.Attachments.Enabled)},
		{Key: "ATTACHMENTS_STORAGE_PATH", Value: cfg.Attachments.StoragePath},
		{Key: "CLAMAV_ADDR", Value: cfg.Attachments.ClamAVAddr},
		{Key: "FEDERATION_BLOCKED_DOMAINS", Value: strings.Join(cfg.Federation.BlockedDomains, ",")},
		{Key: "FEDERATION_ALLOW_INSECURE_HTTP", Value: strconv.FormatBool(cfg.FederationAllowInsecureHTTP())},
		{Key: "FEDERATION_ALLOW_PRIVATE_NETWORKS", Value: strconv.FormatBool(cfg.FederationAllowPrivateNetworks())},
		{Key: "REGISTRATION_ENABLED", Value: strconv.FormatBool(cfg.Registration.Enabled)},
		{Key: "PROJECT_CREATION_POLICY", Value: cfg.Projects.CreationPolicy},
		{Key: "AUTH_RATE_LIMIT_PER_SECOND", Value: strconv.FormatFloat(cfg.RateLimits.Auth.RequestsPerSecond, 'f', -1, 64)},
		{Key: "AUTH_RATE_LIMIT_BURST", Value: strconv.Itoa(cfg.RateLimits.Auth.Burst)},
		{Key: "DISCOVERY_RATE_LIMIT_PER_SECOND", Value: strconv.FormatFloat(cfg.RateLimits.Discovery.RequestsPerSecond, 'f', -1, 64)},
		{Key: "DISCOVERY_RATE_LIMIT_BURST", Value: strconv.Itoa(cfg.RateLimits.Discovery.Burst)},
		{Key: "INBOX_RATE_LIMIT_PER_SECOND", Value: strconv.FormatFloat(cfg.RateLimits.Inbox.RequestsPerSecond, 'f', -1, 64)},
		{Key: "INBOX_RATE_LIMIT_BURST", Value: strconv.Itoa(cfg.RateLimits.Inbox.Burst)},
		{Key: "OAUTH_FRONTEND_CALLBACK_URL", Value: cfg.OAuth.FrontendCallbackURL},
		{Key: "GOOGLE_OAUTH_CLIENT_ID", Value: cfg.OAuth.Google.ClientID},
		{Key: "GOOGLE_OAUTH_CLIENT_SECRET", Value: cfg.OAuth.Google.ClientSecret},
		{Key: "GOOGLE_OAUTH_REDIRECT_URL", Value: cfg.OAuth.Google.RedirectURL},
		{Key: "GITHUB_OAUTH_CLIENT_ID", Value: cfg.OAuth.GitHub.ClientID},
		{Key: "GITHUB_OAUTH_CLIENT_SECRET", Value: cfg.OAuth.GitHub.ClientSecret},
		{Key: "GITHUB_OAUTH_REDIRECT_URL", Value: cfg.OAuth.GitHub.RedirectURL},
		{Key: "GITHUB_API_TOKEN", Value: cfg.GitHub.APIToken},
		{Key: "GITHUB_WEBHOOK_SECRET", Value: cfg.GitHub.WebhookSecret},
		{Key: "FEDERATION_NETWORK", Value: "pms-federation"},
	}
	if options.ImagePrefix != "" {
		entries = append(entries, dotenvEntry{Key: "IMAGE_PREFIX", Value: options.ImagePrefix})
	}
	if options.ImageTag != "" {
		entries = append(entries, dotenvEntry{Key: "IMAGE_TAG", Value: options.ImageTag})
	}

	var builder strings.Builder
	builder.WriteString("# Progo Docker Compose environment generated by pmsctl config export-env.\n")
	for _, entry := range entries {
		builder.WriteString(entry.Key)
		builder.WriteByte('=')
		builder.WriteString(formatDotenvValue(entry.Value))
		builder.WriteByte('\n')
	}
	return []byte(builder.String()), nil
}

// postgresEnvValues contains database settings needed by the Compose db service.
type postgresEnvValues struct {
	User     string
	Password string
	Database string
}

// postgresEnv extracts Compose database settings from a PostgreSQL URL.
func postgresEnv(source string) (postgresEnvValues, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil {
		return postgresEnvValues{}, err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return postgresEnvValues{}, fmt.Errorf("database.source must be a postgres URL")
	}
	user := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	database := strings.TrimPrefix(parsed.Path, "/")
	if user == "" {
		return postgresEnvValues{}, fmt.Errorf("database.source must include a user")
	}
	if !hasPassword || password == "" {
		return postgresEnvValues{}, fmt.Errorf("database.source must include a password")
	}
	if database == "" {
		return postgresEnvValues{}, fmt.Errorf("database.source must include a database name")
	}
	return postgresEnvValues{User: user, Password: password, Database: database}, nil
}

// composeDeployName returns a container-safe deployment name.
func composeDeployName(cfg appconfig.Config, explicit string) string {
	if explicit != "" {
		return slugifyDeployName(explicit)
	}
	if cfg.Instance.LocalDomain != "" {
		return slugifyDeployName(hostWithoutPort(cfg.Instance.LocalDomain))
	}
	return slugifyDeployName(cfg.Instance.Name)
}

// hostWithoutPort removes a port from a host when possible.
func hostWithoutPort(host string) string {
	host = strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}
	if strings.Count(host, ":") == 1 {
		if before, _, ok := strings.Cut(host, ":"); ok {
			return before
		}
	}
	return strings.Trim(host, "[]")
}

// slugifyDeployName makes a conservative Docker resource name token.
func slugifyDeployName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	lastDash := false
	for _, char := range raw {
		allowed := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')
		if allowed {
			builder.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

// formatDotenvValue quotes values only when Compose dotenv parsing needs it.
func formatDotenvValue(value string) string {
	if dotenvRawValuePattern.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// fileExists reports whether a filesystem path already exists.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// generateSecret returns a URL-safe random secret with byteCount bytes of entropy.
func generateSecret(byteCount int) (string, error) {
	secret := make([]byte, byteCount)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}

// defaultPublicBaseURL returns the public URL default for an environment.
func defaultPublicBaseURL(appEnv string) string {
	if appEnv == appconfig.EnvProduction {
		return "https://progo.example.com"
	}
	return "http://localhost:8080"
}

// defaultLocalDomain returns the ActivityPub local domain default.
func defaultLocalDomain(publicBaseURL string, appEnv string) string {
	parsed, err := url.Parse(publicBaseURL)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	if appEnv == appconfig.EnvProduction {
		return "progo.example.com"
	}
	return "localhost:8080"
}

// defaultDatabaseSource returns a generated PostgreSQL source string.
func defaultDatabaseSource(appEnv string, generate func(int) (string, error)) (string, error) {
	password, err := generate(18)
	if err != nil {
		return "", fmt.Errorf("generate database password: %w", err)
	}
	if appEnv == appconfig.EnvProduction {
		return (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword("progo", password),
			Host:     "db:5432",
			Path:     "/progo",
			RawQuery: "sslmode=disable",
		}).String(), nil
	}
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("postgres", password),
		Host:     "localhost:5432",
		Path:     "/pms",
		RawQuery: "sslmode=disable",
	}).String(), nil
}

// defaultCORSOrigins returns browser origins for the chosen environment.
func defaultCORSOrigins(appEnv string, publicBaseURL string) []string {
	if appEnv == appconfig.EnvProduction {
		return []string{strings.TrimRight(publicBaseURL, "/")}
	}
	return []string{"http://localhost:5173"}
}

// defaultFrontendCallbackURL returns the SPA OAuth callback URL.
func defaultFrontendCallbackURL(appEnv string, publicBaseURL string) string {
	if appEnv == appconfig.EnvProduction {
		return joinPublicURL(publicBaseURL, "/oauth/callback")
	}
	return "http://localhost:5173/oauth/callback"
}

// joinPublicURL appends path to the configured public base URL.
func joinPublicURL(publicBaseURL string, path string) string {
	parsed, err := url.Parse(publicBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(publicBaseURL, "/") + path
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

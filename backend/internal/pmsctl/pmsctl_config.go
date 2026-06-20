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
	"net/url"
	"os"
	"strings"

	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"gopkg.in/yaml.v3"
)

// configInitOptions carries validated config init flags.
type configInitOptions struct {
	Output string
	Force  bool
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

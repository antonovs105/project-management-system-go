// Package pmsctl implements the backend maintenance CLI.
package pmsctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Runner executes pmsctl commands with injectable IO and side effects.
type Runner struct {
	Stdin               io.Reader
	Stdout              io.Writer
	Stderr              io.Writer
	LoadEnvFile         func(path string) error
	CreateOwner         func(ctx context.Context, options OwnerCreateOptions) (*user.User, error)
	DiscoverRemoteActor func(ctx context.Context, options FederationDiscoverOptions) (*remoteactor.Actor, error)
	FollowRemoteActor   func(ctx context.Context, options FederationFollowOptions) (*FederationFollowResult, error)
	AcceptProjectFollow func(ctx context.Context, options FederationAcceptFollowOptions) (*FederationAcceptFollowResult, error)
}

// OwnerCreateOptions carries validated owner bootstrap input.
type OwnerCreateOptions struct {
	EnvFile  string
	Username string
	Email    string
	Password string
}

// RuntimeConfig is the environment needed for DB-backed maintenance commands.
type RuntimeConfig struct {
	AppEnv                         string
	DBSource                       string
	PublicBaseURL                  string
	LocalDomain                    string
	ActorPrivateKeyEncryptionKey   string
	FederationAllowInsecureHTTP    bool
	FederationAllowPrivateNetworks bool
}

// defaultEnvFile is the conventional env file loaded by CLI commands.
const defaultEnvFile = ".env"

// NewRunner returns the production pmsctl runner.
func NewRunner() *Runner {
	return &Runner{
		Stdin:               os.Stdin,
		Stdout:              os.Stdout,
		Stderr:              os.Stderr,
		LoadEnvFile:         loadEnvFile,
		CreateOwner:         createOwner,
		DiscoverRemoteActor: discoverRemoteActor,
		FollowRemoteActor:   followRemoteActor,
		AcceptProjectFollow: acceptProjectFollow,
	}
}

// Run dispatches a pmsctl command and returns a process exit code.
func (r *Runner) Run(ctx context.Context, args []string) int {
	r.withDefaults()
	if len(args) == 0 {
		r.printRootUsage()
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		r.printRootUsage()
		return 0
	case "owner":
		return r.runOwner(ctx, args[1:])
	case "federation":
		return r.runFederation(ctx, args[1:])
	default:
		fmt.Fprintf(r.Stderr, "unknown command %q\n\n", args[0])
		r.printRootUsage()
		return 2
	}
}

// withDefaults fills missing runner dependencies with production defaults.
func (r *Runner) withDefaults() {
	if r.Stdin == nil {
		r.Stdin = os.Stdin
	}
	if r.Stdout == nil {
		r.Stdout = os.Stdout
	}
	if r.Stderr == nil {
		r.Stderr = os.Stderr
	}
	if r.LoadEnvFile == nil {
		r.LoadEnvFile = loadEnvFile
	}
	if r.CreateOwner == nil {
		r.CreateOwner = createOwner
	}
	if r.DiscoverRemoteActor == nil {
		r.DiscoverRemoteActor = discoverRemoteActor
	}
	if r.FollowRemoteActor == nil {
		r.FollowRemoteActor = followRemoteActor
	}
	if r.AcceptProjectFollow == nil {
		r.AcceptProjectFollow = acceptProjectFollow
	}
}

// runOwner dispatches owner maintenance subcommands.
func (r *Runner) runOwner(ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.printOwnerUsage()
		return 2
	}
	switch args[0] {
	case "create":
		return r.runOwnerCreate(ctx, args[1:])
	case "help", "-h", "--help":
		r.printOwnerUsage()
		return 0
	default:
		fmt.Fprintf(r.Stderr, "unknown owner command %q\n\n", args[0])
		r.printOwnerUsage()
		return 2
	}
}

// runOwnerCreate parses input and creates the first owner account.
func (r *Runner) runOwnerCreate(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("pmsctl owner create", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	envFile := fs.String("env-file", defaultEnvFile, "environment file to load before connecting")
	username := fs.String("username", "", "owner username")
	email := fs.String("email", "", "owner email")
	password := fs.String("password", "", "owner password")
	passwordStdin := fs.Bool("password-stdin", false, "read owner password from stdin")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl owner create --username USER --email EMAIL (--password PASS | --password-stdin)")
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
	if *passwordStdin {
		if strings.TrimSpace(*password) != "" {
			fmt.Fprintln(r.Stderr, "--password and --password-stdin cannot be used together")
			return 2
		}
		raw, err := io.ReadAll(r.Stdin)
		if err != nil {
			fmt.Fprintf(r.Stderr, "failed to read password from stdin: %v\n", err)
			return 1
		}
		*password = strings.TrimRight(string(raw), "\r\n")
	}

	options := OwnerCreateOptions{
		EnvFile:  strings.TrimSpace(*envFile),
		Username: strings.TrimSpace(*username),
		Email:    strings.TrimSpace(*email),
		Password: *password,
	}
	if err := validateOwnerCreateOptions(options); err != nil {
		fmt.Fprintf(r.Stderr, "%v\n\n", err)
		fs.Usage()
		return 2
	}
	if err := r.LoadEnvFile(options.EnvFile); err != nil {
		fmt.Fprintf(r.Stderr, "failed to load env file: %v\n", err)
		return 1
	}

	owner, err := r.CreateOwner(ctx, options)
	if err != nil {
		if errors.Is(err, user.ErrAdminAlreadyExists) {
			fmt.Fprintln(r.Stderr, "owner already exists")
			return 1
		}
		fmt.Fprintf(r.Stderr, "failed to create owner: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.Stdout, "owner_created id=%s username=%s email=%s handle=%s\n", owner.ID, owner.Username, owner.Email, owner.Handle)
	return 0
}

// validateOwnerCreateOptions checks required owner bootstrap flags.
func validateOwnerCreateOptions(options OwnerCreateOptions) error {
	missing := make([]string, 0, 3)
	if options.Username == "" {
		missing = append(missing, "--username")
	}
	if options.Email == "" {
		missing = append(missing, "--email")
	}
	if options.Password == "" {
		missing = append(missing, "--password or --password-stdin")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required option(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// loadEnvFile loads optional dotenv configuration before command execution.
func loadEnvFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		if path == defaultEnvFile && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

// createOwner creates the first owner using configured database and ActivityPub settings.
func createOwner(ctx context.Context, options OwnerCreateOptions) (*user.User, error) {
	cfg, err := LoadRuntimeConfig()
	if err != nil {
		return nil, err
	}
	apConfig := activitypub.NewConfig(cfg.PublicBaseURL, cfg.LocalDomain)
	privateKeyCodec, err := secrets.NewPrivateKeyCodec(cfg.ActorPrivateKeyEncryptionKey)
	if err != nil {
		return nil, err
	}

	db, err := sqlx.Open("postgres", cfg.DBSource)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	service := user.NewService(user.NewRepository(db, apConfig, privateKeyCodec), []byte("pmsctl"), apConfig)
	return service.BootstrapAdmin(ctx, options.Username, options.Email, options.Password)
}

// LoadRuntimeConfig reads and validates the environment shared by API and CLI commands.
func LoadRuntimeConfig() (RuntimeConfig, error) {
	cfg := RuntimeConfig{
		AppEnv:                       strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))),
		DBSource:                     strings.TrimSpace(os.Getenv("DB_SOURCE")),
		PublicBaseURL:                strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		LocalDomain:                  strings.TrimSpace(os.Getenv("LOCAL_DOMAIN")),
		ActorPrivateKeyEncryptionKey: strings.TrimSpace(os.Getenv("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY")),
	}
	if cfg.AppEnv == "" {
		cfg.AppEnv = "development"
	}

	missing := make([]string, 0, 3)
	if cfg.DBSource == "" {
		missing = append(missing, "DB_SOURCE")
	}
	if cfg.PublicBaseURL == "" {
		missing = append(missing, "PUBLIC_BASE_URL")
	}
	if cfg.LocalDomain == "" {
		missing = append(missing, "LOCAL_DOMAIN")
	}
	if len(missing) > 0 {
		return RuntimeConfig{}, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	if cfg.AppEnv != "development" && cfg.AppEnv != "test" && cfg.AppEnv != "production" {
		return RuntimeConfig{}, fmt.Errorf("APP_ENV must be one of development, test, or production")
	}
	if value, ok, err := optionalBoolEnv("FEDERATION_ALLOW_INSECURE_HTTP"); err != nil {
		return RuntimeConfig{}, err
	} else if ok {
		cfg.FederationAllowInsecureHTTP = value
	} else {
		cfg.FederationAllowInsecureHTTP = cfg.AppEnv != "production"
	}
	if value, ok, err := optionalBoolEnv("FEDERATION_ALLOW_PRIVATE_NETWORKS"); err != nil {
		return RuntimeConfig{}, err
	} else if ok {
		cfg.FederationAllowPrivateNetworks = value
	}

	parsedBaseURL, err := url.Parse(cfg.PublicBaseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return RuntimeConfig{}, fmt.Errorf("PUBLIC_BASE_URL must be an absolute HTTP URL")
	}
	if parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https" {
		return RuntimeConfig{}, fmt.Errorf("PUBLIC_BASE_URL must use http or https")
	}
	if strings.ContainsAny(cfg.LocalDomain, " \t\r\n/") {
		return RuntimeConfig{}, fmt.Errorf("LOCAL_DOMAIN must be a host name, not a URL")
	}

	if cfg.AppEnv == "production" {
		if parsedBaseURL.Scheme != "https" {
			return RuntimeConfig{}, fmt.Errorf("PUBLIC_BASE_URL must use https in production")
		}
		if isLocalHost(parsedBaseURL.Hostname()) || isLocalHost(cfg.LocalDomain) {
			return RuntimeConfig{}, fmt.Errorf("PUBLIC_BASE_URL and LOCAL_DOMAIN must not use localhost in production")
		}
		if len(cfg.ActorPrivateKeyEncryptionKey) < 32 {
			return RuntimeConfig{}, fmt.Errorf("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY must be at least 32 characters in production")
		}
		if cfg.FederationAllowInsecureHTTP {
			return RuntimeConfig{}, fmt.Errorf("FEDERATION_ALLOW_INSECURE_HTTP cannot be enabled in production")
		}
		if cfg.FederationAllowPrivateNetworks {
			return RuntimeConfig{}, fmt.Errorf("FEDERATION_ALLOW_PRIVATE_NETWORKS cannot be enabled in production")
		}
	}

	return cfg, nil
}

// optionalBoolEnv parses an optional boolean environment setting.
func optionalBoolEnv(name string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, false, nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, true, nil
	case "0", "false", "no", "off":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("%s must be a boolean", name)
	}
}

// isLocalHost reports whether host points to the loopback development host.
func isLocalHost(host string) bool {
	host = strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// printRootUsage writes the top-level command help.
func (r *Runner) printRootUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  owner create    Create the first local owner account")
	fmt.Fprintln(r.Stderr, "  federation      Discover and send local federation activities")
}

// printOwnerUsage writes owner command help.
func (r *Runner) printOwnerUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl owner <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  create          Create the first local owner account")
}

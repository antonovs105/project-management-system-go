// Package pmsctl implements the backend maintenance CLI.
package pmsctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
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
	LoadAppConfig       func(path string) (appconfig.Config, error)
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
		LoadAppConfig:       loadAppConfig,
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
	case "config":
		return r.runConfig(ctx, args[1:])
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
	if r.LoadAppConfig == nil {
		r.LoadAppConfig = loadAppConfig
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

// runConfig dispatches local configuration subcommands.
func (r *Runner) runConfig(ctx context.Context, args []string) int {
	if len(args) == 0 {
		r.printConfigUsage()
		return 2
	}
	switch args[0] {
	case "validate":
		return r.runConfigValidate(ctx, args[1:])
	case "help", "-h", "--help":
		r.printConfigUsage()
		return 0
	default:
		fmt.Fprintf(r.Stderr, "unknown config command %q\n\n", args[0])
		r.printConfigUsage()
		return 2
	}
}

// runConfigValidate loads dotenv/YAML config and reports validation status.
func (r *Runner) runConfigValidate(ctx context.Context, args []string) int {
	_ = ctx
	fs := flag.NewFlagSet("pmsctl config validate", flag.ContinueOnError)
	fs.SetOutput(r.Stderr)
	envFile := fs.String("env-file", defaultEnvFile, "environment file to load before validating")
	configFile := fs.String("config", "", "YAML config file to validate")
	fs.Usage = func() {
		fmt.Fprintln(r.Stderr, "Usage: pmsctl config validate [--config FILE] [--env-file FILE]")
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
	if err := r.LoadEnvFile(strings.TrimSpace(*envFile)); err != nil {
		fmt.Fprintf(r.Stderr, "failed to load env file: %v\n", err)
		return 1
	}
	cfg, err := r.LoadAppConfig(strings.TrimSpace(*configFile))
	if err != nil {
		fmt.Fprintf(r.Stderr, "config_invalid error=%q\n", err.Error())
		return 1
	}
	fmt.Fprintf(
		r.Stdout,
		"config_valid app_env=%s role=%s registration_enabled=%t project_creation_policy=%s\n",
		cfg.App.Env,
		cfg.App.Role,
		cfg.Registration.Enabled,
		cfg.Projects.CreationPolicy,
	)
	return 0
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

// loadAppConfig validates either the default runtime config or an explicit YAML path.
func loadAppConfig(path string) (appconfig.Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return appconfig.Load()
	}
	return appconfig.LoadFile(path)
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
	cfg, err := appconfig.Load()
	if err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{
		AppEnv:                         cfg.App.Env,
		DBSource:                       cfg.Database.Source,
		PublicBaseURL:                  cfg.Instance.PublicBaseURL,
		LocalDomain:                    cfg.Instance.LocalDomain,
		ActorPrivateKeyEncryptionKey:   cfg.Security.ActorPrivateKeyEncryptionKey,
		FederationAllowInsecureHTTP:    cfg.FederationAllowInsecureHTTP(),
		FederationAllowPrivateNetworks: cfg.FederationAllowPrivateNetworks(),
	}, nil
}

// printRootUsage writes the top-level command help.
func (r *Runner) printRootUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  owner create    Create the first local owner account")
	fmt.Fprintln(r.Stderr, "  federation      Discover and send local federation activities")
	fmt.Fprintln(r.Stderr, "  config validate Validate runtime configuration")
}

// printOwnerUsage writes owner command help.
func (r *Runner) printOwnerUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl owner <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  create          Create the first local owner account")
}

// printConfigUsage writes configuration command help.
func (r *Runner) printConfigUsage() {
	fmt.Fprintln(r.Stderr, "Usage: pmsctl config <command> [options]")
	fmt.Fprintln(r.Stderr)
	fmt.Fprintln(r.Stderr, "Commands:")
	fmt.Fprintln(r.Stderr, "  validate        Validate dotenv/YAML runtime configuration")
}

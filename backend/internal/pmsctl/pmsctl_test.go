package pmsctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	appconfig "github.com/antonovs105/project-management-system-go/internal/config"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOwnerRecoverRequiresConfirmationAndUsesPasswordStdin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var captured OwnerRecoverOptions
	runner := Runner{
		Stdin:  strings.NewReader("replacement123\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			require.Equal(t, "recovery.env", path)
			return nil
		},
		RecoverOwner: func(ctx context.Context, options OwnerRecoverOptions) (*account.OwnerRecoveryResult, error) {
			captured = options
			return &account.OwnerRecoveryResult{UserID: "owner-id", Username: "owner", MFAReset: true}, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "recover",
		"--env-file", "recovery.env",
		"--username", "owner",
		"--confirm-username", "owner",
		"--password-stdin",
		"--reset-mfa",
	})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(t, OwnerRecoverOptions{
		EnvFile:         "recovery.env",
		Username:        "owner",
		ConfirmUsername: "owner",
		Password:        "replacement123",
		ResetMFA:        true,
	}, captured)
	require.Contains(t, stdout.String(), "owner_recovered")
	require.Contains(t, stdout.String(), "sessions_revoked=true")
	require.Contains(t, stdout.String(), "mfa_reset=true")
}

func TestOwnerRecoverRejectsConfirmationMismatch(t *testing.T) {
	var stderr bytes.Buffer
	called := false
	runner := Runner{
		Stdin:  strings.NewReader("replacement123\n"),
		Stderr: &stderr,
		RecoverOwner: func(ctx context.Context, options OwnerRecoverOptions) (*account.OwnerRecoveryResult, error) {
			called = true
			return nil, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "recover",
		"--username", "owner",
		"--confirm-username", "different-owner",
		"--password-stdin",
	})

	require.Equal(t, 2, code)
	require.False(t, called)
	require.Contains(t, stderr.String(), "must exactly match")
}

func TestOwnerCreateReadsPasswordFromStdin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var loadedEnvFile string
	var captured OwnerCreateOptions
	runner := Runner{
		Stdin:  strings.NewReader("password123\n"),
		Stdout: &stdout,
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			loadedEnvFile = path
			return nil
		},
		CreateOwner: func(ctx context.Context, options OwnerCreateOptions) (*user.User, error) {
			captured = options
			return &user.User{
				ID:       "owner-id",
				Username: "owner",
				Email:    "owner@example.test",
				Handle:   "owner@alpha.pms.test",
			}, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "create",
		"--env-file", "alpha.env",
		"--username", "owner",
		"--email", "owner@example.test",
		"--password-stdin",
	})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "alpha.env", loadedEnvFile)
	require.Equal(t, OwnerCreateOptions{
		EnvFile:  "alpha.env",
		Username: "owner",
		Email:    "owner@example.test",
		Password: "password123",
	}, captured)
	require.Contains(t, stdout.String(), "owner_created")
	require.Contains(t, stdout.String(), "handle=owner@alpha.pms.test")
}

func TestOwnerCreateRejectsMissingPassword(t *testing.T) {
	var stderr bytes.Buffer
	called := false
	runner := Runner{
		Stderr: &stderr,
		CreateOwner: func(ctx context.Context, options OwnerCreateOptions) (*user.User, error) {
			called = true
			return nil, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "create",
		"--username", "owner",
		"--email", "owner@example.test",
	})

	require.Equal(t, 2, code)
	require.False(t, called)
	require.Contains(t, stderr.String(), "--password or --password-stdin")
}

func TestOwnerCreateMapsExistingOwner(t *testing.T) {
	var stderr bytes.Buffer
	runner := Runner{
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			return nil
		},
		CreateOwner: func(ctx context.Context, options OwnerCreateOptions) (*user.User, error) {
			return nil, user.ErrAdminAlreadyExists
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "create",
		"--username", "owner",
		"--email", "owner@example.test",
		"--password", "password123",
	})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "owner already exists")
}

func TestOwnerCreateMapsEnvFileError(t *testing.T) {
	var stderr bytes.Buffer
	runner := Runner{
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			return errors.New("missing env")
		},
	}

	code := runner.Run(context.Background(), []string{
		"owner", "create",
		"--username", "owner",
		"--email", "owner@example.test",
		"--password", "password123",
	})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "failed to load env file")
}

func TestConfigValidateLoadsEnvAndReportsPolicy(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var loadedEnvFile string
	var loadedConfigFile string
	runner := Runner{
		Stdout: &stdout,
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			loadedEnvFile = path
			return nil
		},
		LoadAppConfig: func(path string) (appconfig.Config, error) {
			loadedConfigFile = path
			cfg := appconfig.Default()
			cfg.Database.Source = "postgres://postgres:postgres@db:5432/pms?sslmode=disable"
			cfg.Security.JWTSecretKey = "dev-secret"
			cfg.Instance.PublicBaseURL = "http://localhost:8080"
			cfg.Instance.LocalDomain = "localhost:8080"
			cfg.Registration.Enabled = false
			cfg.Projects.CreationPolicy = appconfig.ProjectCreationAdminsOnly
			return cfg, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"config", "validate",
		"--env-file", "alpha.env",
		"--config", "progo.yml",
	})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "alpha.env", loadedEnvFile)
	require.Equal(t, "progo.yml", loadedConfigFile)
	require.Contains(t, stdout.String(), "config_valid")
	require.Contains(t, stdout.String(), "registration_enabled=false")
	require.Contains(t, stdout.String(), "project_creation_policy=admins_only")
}

func TestConfigValidateReportsInvalidConfig(t *testing.T) {
	var stderr bytes.Buffer
	runner := Runner{
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			return nil
		},
		LoadAppConfig: func(path string) (appconfig.Config, error) {
			return appconfig.Config{}, errors.New("bad config")
		},
	}

	code := runner.Run(context.Background(), []string{"config", "validate"})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "config_invalid")
	require.Contains(t, stderr.String(), "bad config")
}

func TestConfigInitWritesValidatedProductionConfig(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	output := filepath.Join(t.TempDir(), "progo.yml")
	runner := Runner{
		Stdin: strings.NewReader(strings.Join([]string{
			"production",
			"Alpha",
			"https://alpha.example.test",
			"",
			"",
			"redis:6379",
			"",
			"n",
			"admins_only",
			"n",
			"n",
		}, "\n") + "\n"),
		Stdout:         &stdout,
		Stderr:         &stderr,
		GenerateSecret: sequentialSecretGenerator(),
	}

	code := runner.Run(context.Background(), []string{"config", "init", "--output", output})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "config_initialized")
	require.Contains(t, stdout.String(), "registration_enabled=false")
	require.Contains(t, stdout.String(), "project_creation_policy=admins_only")

	cfg, raw := readGeneratedConfig(t, output)
	require.Equal(t, appconfig.EnvProduction, cfg.App.Env)
	require.Equal(t, appconfig.RoleAll, cfg.App.Role)
	require.Equal(t, "Alpha", cfg.Instance.Name)
	require.Equal(t, "https://alpha.example.test", cfg.Instance.PublicBaseURL)
	require.Equal(t, "alpha.example.test", cfg.Instance.LocalDomain)
	require.False(t, cfg.Registration.Enabled)
	require.Equal(t, appconfig.ProjectCreationAdminsOnly, cfg.Projects.CreationPolicy)
	require.Equal(t, []string{"https://alpha.example.test"}, cfg.Server.CORSAllowedOrigins)
	require.Equal(t, "redis:6379", cfg.Redis.Addr)
	require.False(t, cfg.FederationAllowInsecureHTTP())
	require.False(t, cfg.FederationAllowPrivateNetworks())
	require.Len(t, cfg.Security.JWTSecretKey, 32)
	require.Len(t, cfg.Security.ActorPrivateKeyEncryptionKey, 32)
	require.Len(t, cfg.Metrics.Token, 32)
	require.Contains(t, cfg.Database.Source, "postgres://progo:")
	require.NotContains(t, string(raw), "change-me")
}

func TestConfigInitRefusesOverwriteWithoutForce(t *testing.T) {
	var stderr bytes.Buffer
	output := filepath.Join(t.TempDir(), "progo.yml")
	require.NoError(t, os.WriteFile(output, []byte("old"), 0o600))
	runner := Runner{
		Stdin:  strings.NewReader(""),
		Stderr: &stderr,
	}

	code := runner.Run(context.Background(), []string{"config", "init", "--output", output})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "already exists")
	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "old", string(raw))
}

func TestConfigInitForceOverwritesExistingFile(t *testing.T) {
	var stdout bytes.Buffer
	output := filepath.Join(t.TempDir(), "progo.yml")
	require.NoError(t, os.WriteFile(output, []byte("old"), 0o600))
	runner := Runner{
		Stdin:          strings.NewReader(strings.Repeat("\n", 11)),
		Stdout:         &stdout,
		GenerateSecret: sequentialSecretGenerator(),
	}

	code := runner.Run(context.Background(), []string{"config", "init", "--output", output, "--force"})

	require.Equal(t, 0, code)
	require.Contains(t, stdout.String(), "config_initialized")
	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	require.NotEqual(t, "old", string(raw))
	require.Contains(t, string(raw), "registration:")
}

func TestConfigInitRejectsInvalidGeneratedProductionConfig(t *testing.T) {
	var stderr bytes.Buffer
	output := filepath.Join(t.TempDir(), "progo.yml")
	runner := Runner{
		Stdin: strings.NewReader(strings.Join([]string{
			"production",
			"Alpha",
			"https://alpha.example.test",
			"",
			"",
			"redis:6379",
			"",
			"y",
			"admins_only",
			"y",
			"n",
		}, "\n") + "\n"),
		Stderr:         &stderr,
		GenerateSecret: sequentialSecretGenerator(),
	}

	code := runner.Run(context.Background(), []string{"config", "init", "--output", output})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "generated config is invalid")
	require.Contains(t, stderr.String(), "federation.allow_insecure_http")
	_, err := os.Stat(output)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestConfigInitFailsWhenSecretGenerationFails(t *testing.T) {
	var stderr bytes.Buffer
	output := filepath.Join(t.TempDir(), "progo.yml")
	runner := Runner{
		Stdin: strings.NewReader(strings.Join([]string{
			"production",
			"Alpha",
			"https://alpha.example.test",
			"",
		}, "\n") + "\n"),
		Stderr: &stderr,
		GenerateSecret: func(byteCount int) (string, error) {
			return "", errors.New("entropy unavailable")
		},
	}

	code := runner.Run(context.Background(), []string{"config", "init", "--output", output})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "generate database password")
	require.Contains(t, stderr.String(), "entropy unavailable")
	_, err := os.Stat(output)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestConfigExportEnvWritesComposeEnvWithoutEnvironmentOverrides(t *testing.T) {
	disabled := false
	cfg := appconfig.Default()
	cfg.App.Env = appconfig.EnvProduction
	cfg.Database.Source = "postgres://progo:database-password@db:5432/progo?sslmode=disable"
	cfg.Security.JWTSecretKey = strings.Repeat("j", 32)
	cfg.Security.ActorPrivateKeyEncryptionKey = strings.Repeat("a", 32)
	cfg.Instance.Name = "Alpha Progo"
	cfg.Instance.PublicBaseURL = "https://alpha.example.test"
	cfg.Instance.LocalDomain = "alpha.example.test"
	cfg.Server.CORSAllowedOrigins = []string{"https://alpha.example.test"}
	cfg.Server.TrustedProxyCIDRs = []string{"127.0.0.1/32"}
	cfg.Registration.Enabled = false
	cfg.Projects.CreationPolicy = appconfig.ProjectCreationAdminsOnly
	cfg.RateLimits.Auth.RequestsPerSecond = 3.5
	cfg.RateLimits.Auth.Burst = 7
	cfg.Metrics.Token = strings.Repeat("m", 32)
	cfg.Federation.BlockedDomains = []string{"blocked.example.test"}
	cfg.Federation.AllowInsecureHTTP = &disabled
	cfg.OAuth.Google.ClientID = "google-client"
	cfg.GitHub.WebhookSecret = "github webhook secret"

	configPath := writeRuntimeConfig(t, cfg)
	output := filepath.Join(t.TempDir(), ".env")
	t.Setenv("PUBLIC_BASE_URL", "https://env.example.test")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{Stdout: &stdout, Stderr: &stderr}

	code := runner.Run(context.Background(), []string{
		"config", "export-env",
		"--config", configPath,
		"--output", output,
		"--image-prefix", "ghcr.io/example/progo",
		"--image-tag", "abc123",
	})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "dotenv_exported")
	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "INSTANCE_NAME=alpha-example-test\n")
	require.Contains(t, body, "APP_ENV=production\n")
	require.Contains(t, body, "POSTGRES_USER=progo\n")
	require.Contains(t, body, "POSTGRES_PASSWORD=database-password\n")
	require.Contains(t, body, "POSTGRES_DB=progo\n")
	require.Contains(t, body, "PUBLIC_BASE_URL=https://alpha.example.test\n")
	require.NotContains(t, body, "https://env.example.test")
	require.Contains(t, body, "REGISTRATION_ENABLED=false\n")
	require.Contains(t, body, "PROJECT_CREATION_POLICY=admins_only\n")
	require.Contains(t, body, "AUTH_RATE_LIMIT_PER_SECOND=3.5\n")
	require.Contains(t, body, "AUTH_RATE_LIMIT_BURST=7\n")
	require.Contains(t, body, "FEDERATION_BLOCKED_DOMAINS=blocked.example.test\n")
	require.Contains(t, body, "GOOGLE_OAUTH_CLIENT_ID=google-client\n")
	require.Contains(t, body, "GITHUB_WEBHOOK_SECRET='github webhook secret'\n")
	require.Contains(t, body, "IMAGE_PREFIX=ghcr.io/example/progo\n")
	require.Contains(t, body, "IMAGE_TAG=abc123\n")
}

func TestConfigExportEnvRefusesOverwriteWithoutForce(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Database.Source = "postgres://postgres:postgres@localhost:5432/pms?sslmode=disable"
	cfg.Security.JWTSecretKey = "dev-secret"
	cfg.Instance.PublicBaseURL = "http://localhost:8080"
	cfg.Instance.LocalDomain = "localhost:8080"
	configPath := writeRuntimeConfig(t, cfg)
	output := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(output, []byte("old"), 0o600))
	var stderr bytes.Buffer
	runner := Runner{Stderr: &stderr}

	code := runner.Run(context.Background(), []string{"config", "export-env", "--config", configPath, "--output", output})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "already exists")
	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "old", string(raw))
}

func TestConfigExportEnvRejectsDatabaseSourceWithoutPassword(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Database.Source = "postgres://postgres@localhost:5432/pms?sslmode=disable"
	cfg.Security.JWTSecretKey = "dev-secret"
	cfg.Instance.PublicBaseURL = "http://localhost:8080"
	cfg.Instance.LocalDomain = "localhost:8080"
	configPath := writeRuntimeConfig(t, cfg)
	output := filepath.Join(t.TempDir(), ".env")
	var stderr bytes.Buffer
	runner := Runner{Stderr: &stderr}

	code := runner.Run(context.Background(), []string{"config", "export-env", "--config", configPath, "--output", output})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "database.source must include a password")
	_, err := os.Stat(output)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestFederationDiscoverUsesPositionalResource(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var loadedEnvFile string
	var captured FederationDiscoverOptions
	runner := Runner{
		Stdout: &stdout,
		Stderr: &stderr,
		LoadEnvFile: func(path string) error {
			loadedEnvFile = path
			return nil
		},
		DiscoverRemoteActor: func(ctx context.Context, options FederationDiscoverOptions) (*remoteactor.Actor, error) {
			captured = options
			return &remoteactor.Actor{
				APID:      "http://beta.test/projects/1",
				Type:      "Group",
				Handle:    "project-1@beta.test",
				InboxURL:  "http://beta.test/projects/1/inbox",
				OutboxURL: "http://beta.test/projects/1/outbox",
			}, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"federation", "discover",
		"--env-file", "alpha.env",
		"project-1@beta.test",
	})

	require.Equal(t, 0, code)
	require.Empty(t, stderr.String())
	require.Equal(t, "alpha.env", loadedEnvFile)
	require.Equal(t, FederationDiscoverOptions{EnvFile: "alpha.env", Resource: "project-1@beta.test"}, captured)
	require.Contains(t, stdout.String(), "actor_discovered")
	require.Contains(t, stdout.String(), "ap_id=http://beta.test/projects/1")
}

func TestFederationFollowRequiresFromUser(t *testing.T) {
	var stderr bytes.Buffer
	called := false
	runner := Runner{
		Stderr: &stderr,
		FollowRemoteActor: func(ctx context.Context, options FederationFollowOptions) (*FederationFollowResult, error) {
			called = true
			return nil, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"federation", "follow",
		"acct:project-1@beta.test",
	})

	require.Equal(t, 2, code)
	require.False(t, called)
	require.Contains(t, stderr.String(), "--from-user")
}

func TestFederationFollowSendsTarget(t *testing.T) {
	var stdout bytes.Buffer
	var captured FederationFollowOptions
	runner := Runner{
		Stdout: &stdout,
		LoadEnvFile: func(path string) error {
			return nil
		},
		FollowRemoteActor: func(ctx context.Context, options FederationFollowOptions) (*FederationFollowResult, error) {
			captured = options
			return &FederationFollowResult{
				ActivityAPID:   "http://alpha.test/activities/follow-1",
				ActorAPID:      "http://alpha.test/users/alice",
				TargetAPID:     "http://beta.test/projects/1",
				TargetInboxURL: "http://beta.test/projects/1/inbox",
				StatusCode:     202,
			}, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"federation", "follow",
		"--from-user", "alice",
		"--target", "acct:project-1@beta.test",
	})

	require.Equal(t, 0, code)
	require.Equal(t, FederationFollowOptions{EnvFile: ".env", FromUser: "alice", Target: "acct:project-1@beta.test"}, captured)
	require.Contains(t, stdout.String(), "follow_sent")
	require.Contains(t, stdout.String(), "status=202")
}

func TestFederationAcceptFollowCapturesOptions(t *testing.T) {
	var stdout bytes.Buffer
	var captured FederationAcceptFollowOptions
	runner := Runner{
		Stdout: &stdout,
		LoadEnvFile: func(path string) error {
			return nil
		},
		AcceptProjectFollow: func(ctx context.Context, options FederationAcceptFollowOptions) (*FederationAcceptFollowResult, error) {
			captured = options
			return &FederationAcceptFollowResult{
				ResponseActivityAPID: "http://beta.test/activities/accept-1",
				ProjectAPID:          "http://beta.test/projects/project-1",
				ActorAPID:            "http://alpha.test/users/alice",
				TargetInboxURL:       "http://alpha.test/users/alice/inbox",
				ResponseStatusCode:   202,
			}, nil
		},
	}

	code := runner.Run(context.Background(), []string{
		"federation", "accept-follow",
		"--project-id", "project-1",
		"--actor", "http://alpha.test/users/alice",
		"--send-response=false",
	})

	require.Equal(t, 0, code)
	require.Equal(t, FederationAcceptFollowOptions{
		EnvFile:      ".env",
		ProjectID:    "project-1",
		Actor:        "http://alpha.test/users/alice",
		SendResponse: false,
	}, captured)
	require.Contains(t, stdout.String(), "follow_accepted")
}

func TestLoadRuntimeConfigValidatesProductionSafety(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_SOURCE", "postgres://postgres:postgres@db:5432/pms?sslmode=disable")
	t.Setenv("JWT_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("PUBLIC_BASE_URL", "http://alpha.pms.test")
	t.Setenv("LOCAL_DOMAIN", "alpha.pms.test")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://alpha.pms.test")
	t.Setenv("METRICS_TOKEN", "metrics-token-0123456789abcdef0123456789")
	t.Setenv("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
	require.Contains(t, err.Error(), "https")
}

func TestLoadRuntimeConfigRejectsPrivateFederationNetworksInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_SOURCE", "postgres://postgres:postgres@db:5432/pms?sslmode=disable")
	t.Setenv("JWT_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("PUBLIC_BASE_URL", "https://alpha.pms.test")
	t.Setenv("LOCAL_DOMAIN", "alpha.pms.test")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://alpha.pms.test")
	t.Setenv("METRICS_TOKEN", "metrics-token-0123456789abcdef0123456789")
	t.Setenv("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("FEDERATION_ALLOW_PRIVATE_NETWORKS", "true")

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
	require.Contains(t, err.Error(), "federation.allow_private_networks")
}

func readGeneratedConfig(t *testing.T, path string) (appconfig.Config, []byte) {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var cfg appconfig.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	appconfig.Normalize(&cfg)
	require.NoError(t, cfg.Validate())

	return cfg, raw
}

func writeRuntimeConfig(t *testing.T, cfg appconfig.Config) string {
	t.Helper()

	data, err := renderConfigYAML(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "progo.yml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func sequentialSecretGenerator() func(int) (string, error) {
	counter := 0
	return func(byteCount int) (string, error) {
		counter++
		digit := fmt.Sprintf("%d", counter%10)
		return strings.Repeat(digit, byteCount), nil
	}
}

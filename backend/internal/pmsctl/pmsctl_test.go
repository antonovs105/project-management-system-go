package pmsctl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/stretchr/testify/require"
)

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

func TestLoadRuntimeConfigValidatesProductionSafety(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_SOURCE", "postgres://postgres:postgres@db:5432/pms?sslmode=disable")
	t.Setenv("PUBLIC_BASE_URL", "http://alpha.pms.test")
	t.Setenv("LOCAL_DOMAIN", "alpha.pms.test")

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
	require.Contains(t, err.Error(), "https")
}

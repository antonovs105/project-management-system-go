package pmsctl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
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
	t.Setenv("PUBLIC_BASE_URL", "http://alpha.pms.test")
	t.Setenv("LOCAL_DOMAIN", "alpha.pms.test")

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
	require.Contains(t, err.Error(), "https")
}

func TestLoadRuntimeConfigRejectsPrivateFederationNetworksInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_SOURCE", "postgres://postgres:postgres@db:5432/pms?sslmode=disable")
	t.Setenv("PUBLIC_BASE_URL", "https://alpha.pms.test")
	t.Setenv("LOCAL_DOMAIN", "alpha.pms.test")
	t.Setenv("ACTOR_PRIVATE_KEY_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("FEDERATION_ALLOW_PRIVATE_NETWORKS", "true")

	_, err := LoadRuntimeConfig()

	require.Error(t, err)
	require.Contains(t, err.Error(), "FEDERATION_ALLOW_PRIVATE_NETWORKS")
}

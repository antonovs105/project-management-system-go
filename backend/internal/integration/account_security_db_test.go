//go:build integration

package integration

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAccountSecurityLifecycleAgainstPostgreSQL(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests against a migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := sqlx.Connect("postgres", source)
	require.NoError(t, err)
	defer db.Close()

	suffix := uuid.NewString()[:8]
	username := "account_" + suffix
	email := username + "@example.test"
	userService := user.NewService(
		user.NewRepository(db, activitypub.NewConfig("http://localhost:8080", "localhost:8080")),
		[]byte("integration-session-secret"),
		activitypub.NewConfig("http://localhost:8080", "localhost:8080"),
	)
	created, err := userService.RegisterUser(ctx, username, email, "password123")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM actors WHERE id = $1`, created.ID) })

	repository := account.NewRepository(db)
	service := account.NewService(repository, "http://localhost:5173", "Progo", nil, true)
	require.NoError(t, service.SendRegistrationVerification(ctx, created.ID, email))
	verificationToken := latestOutboxToken(t, db, email, "/auth/verify-email")
	require.NoError(t, service.VerifyEmail(ctx, verificationToken))
	var verified bool
	require.NoError(t, db.Get(&verified, `SELECT email_verified FROM users WHERE id = $1`, created.ID))
	require.True(t, verified)
	require.ErrorIs(t, service.VerifyEmail(ctx, verificationToken), account.ErrInvalidToken)

	client := account.ClientInfo{IPAddress: "203.0.113.10", UserAgent: "integration-browser"}
	sessionID, err := service.CreateSession(ctx, created.ID, client)
	require.NoError(t, err)
	require.NoError(t, service.ValidateSession(ctx, created.ID, sessionID))
	sessions, err := service.ListSessions(ctx, created.ID, sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, sessions)
	require.True(t, sessions[0].Current)
	require.NoError(t, service.RevokeSession(ctx, created.ID, sessionID, client))
	require.Error(t, service.ValidateSession(ctx, created.ID, sessionID))

	require.NoError(t, service.RequestPasswordReset(ctx, email, client))
	resetToken := latestOutboxToken(t, db, email, "/auth/reset-password")
	require.NoError(t, service.ResetPassword(ctx, resetToken, "replacement123", client))
	var passwordHash string
	require.NoError(t, db.Get(&passwordHash, `SELECT password_hash FROM users WHERE id = $1`, created.ID))
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("replacement123")))
	require.ErrorIs(t, service.ResetPassword(ctx, resetToken, "anotherpass123", client), account.ErrInvalidToken)

	events, err := service.ListAuthEvents(ctx, created.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 4)
}

func latestOutboxToken(t *testing.T, db *sqlx.DB, recipient, path string) string {
	t.Helper()
	var body string
	require.NoError(t, db.Get(&body, `
		SELECT text_body FROM email_outbox
		WHERE recipient = $1 AND text_body LIKE $2
		ORDER BY created_at DESC LIMIT 1
	`, recipient, "%"+path+"%"))
	match := regexp.MustCompile(`https?://[^\s]+`).FindString(body)
	require.NotEmpty(t, match)
	parsed, err := url.Parse(match)
	require.NoError(t, err)
	require.Equal(t, path, parsed.Path)
	token := parsed.Query().Get("token")
	require.NotEmpty(t, token)
	return token
}

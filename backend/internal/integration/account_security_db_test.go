//go:build integration

package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 -- test helper follows RFC 6238.
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/account"
	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
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
	secretCodec, err := secrets.NewPrivateKeyCodec(strings.Repeat("m", 32))
	require.NoError(t, err)
	service := account.NewService(repository, "http://localhost:5173", "Progo", nil, true, account.WithSecretCodec(secretCodec))
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

	setup, err := service.BeginMFA(ctx, created.ID)
	require.NoError(t, err)
	var storedMFASecret string
	require.NoError(t, db.Get(&storedMFASecret, `SELECT secret_ciphertext FROM user_mfa_credentials WHERE user_id = $1`, created.ID))
	require.NotEqual(t, setup.Secret, storedMFASecret)
	require.True(t, secrets.IsEncryptedPrivateKey(storedMFASecret))
	mfaCode := integrationTOTP(t, setup.Secret, time.Now().UTC())
	recovery, err := service.ConfirmMFA(ctx, created.ID, mfaCode, client)
	require.NoError(t, err)
	require.Len(t, recovery.RecoveryCodes, 10)
	require.ErrorIs(t, service.VerifyMFA(ctx, created.ID, ""), account.ErrMFARequired)
	require.NoError(t, service.VerifyMFA(ctx, created.ID, mfaCode))
	require.NoError(t, service.VerifyMFA(ctx, created.ID, recovery.RecoveryCodes[0]))
	require.ErrorIs(t, service.VerifyMFA(ctx, created.ID, recovery.RecoveryCodes[0]), account.ErrMFAInvalid)

	require.NoError(t, service.RequestPasswordReset(ctx, email, client))
	resetToken := latestOutboxToken(t, db, email, "/auth/reset-password")
	require.NoError(t, service.ResetPassword(ctx, resetToken, "replacement123", client))
	var passwordHash string
	require.NoError(t, db.Get(&passwordHash, `SELECT password_hash FROM users WHERE id = $1`, created.ID))
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("replacement123")))
	require.ErrorIs(t, service.ResetPassword(ctx, resetToken, "anotherpass123", client), account.ErrInvalidToken)

	_, err = db.Exec(`UPDATE users SET instance_role = $2 WHERE id = $1`, created.ID, user.InstanceRoleOwner)
	require.NoError(t, err)
	recoverySessionID, err := service.CreateSession(ctx, created.ID, client)
	require.NoError(t, err)
	require.NoError(t, service.RequestPasswordReset(ctx, email, client))
	var previousTokenVersion int
	require.NoError(t, db.Get(&previousTokenVersion, `SELECT token_version FROM users WHERE id = $1`, created.ID))
	recoveryHash, err := bcrypt.GenerateFromPassword([]byte("offline-recovery123"), bcrypt.DefaultCost)
	require.NoError(t, err)
	recovered, err := repository.RecoverOwnerCredentials(ctx, username, string(recoveryHash), true)
	require.NoError(t, err)
	require.Equal(t, created.ID, recovered.UserID)
	require.True(t, recovered.MFAReset)
	require.Error(t, service.ValidateSession(ctx, created.ID, recoverySessionID))
	require.NoError(t, db.Get(&passwordHash, `SELECT password_hash FROM users WHERE id = $1`, created.ID))
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("offline-recovery123")))
	var currentTokenVersion int
	require.NoError(t, db.Get(&currentTokenVersion, `SELECT token_version FROM users WHERE id = $1`, created.ID))
	require.Equal(t, previousTokenVersion+1, currentTokenVersion)
	var activeMFA, activeRecoveryTokens, recoveryEvents int
	require.NoError(t, db.Get(&activeMFA, `SELECT count(*) FROM user_mfa_credentials WHERE user_id = $1`, created.ID))
	require.NoError(t, db.Get(&activeRecoveryTokens, `SELECT count(*) FROM account_tokens WHERE user_id = $1 AND consumed_at IS NULL`, created.ID))
	require.NoError(t, db.Get(&recoveryEvents, `SELECT count(*) FROM auth_events WHERE user_id = $1 AND event_type = 'owner.recovered'`, created.ID))
	require.Zero(t, activeMFA)
	require.Zero(t, activeRecoveryTokens)
	require.Equal(t, 1, recoveryEvents)

	events, err := service.ListAuthEvents(ctx, created.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 4)
	var securityAlerts int
	require.NoError(t, db.Get(&securityAlerts, `
		SELECT count(*) FROM email_outbox
		WHERE recipient = $1 AND subject IN ('New sign-in', 'Password changed', 'Multi-factor authentication enabled')
	`, email))
	require.GreaterOrEqual(t, securityAlerts, 3)
}

// integrationTOTP calculates the six-digit code used by the account service.
func integrationTOTP(t *testing.T, secret string, now time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	require.NoError(t, err)
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(now.Unix()/30))
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
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

package account

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountTokenHashDoesNotPersistRawToken(t *testing.T) {
	raw := "single-use-secret"
	first := hashToken(raw)
	second := hashToken(raw)
	require.Equal(t, first, second)
	require.NotContains(t, first, raw)
	require.Len(t, first, 64)
}

func TestAccountPasswordPolicyMatchesBcryptBoundary(t *testing.T) {
	require.ErrorIs(t, validatePassword("short"), ErrInvalidPassword)
	require.NoError(t, validatePassword("correct horse battery staple"))
	require.ErrorIs(t, validatePassword(strings.Repeat("x", 73)), ErrInvalidPassword)
}

func TestSMTPConfigAndHeaderSanitization(t *testing.T) {
	_, err := NewSMTPMailer(SMTPConfig{})
	require.Error(t, err)
	mailer, err := NewSMTPMailer(SMTPConfig{Host: "smtp.example.test", Port: 587, FromAddress: "progo@example.test"})
	require.NoError(t, err)
	require.NotNil(t, mailer)
	require.Equal(t, "subject  Bcc: victim@example.test", sanitizeHeader("subject\r\nBcc: victim@example.test"))
}

func TestRandomAccountTokensAreURLSafeAndUnique(t *testing.T) {
	first, err := randomToken(32)
	require.NoError(t, err)
	second, err := randomToken(32)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NotContains(t, first, "=")
}

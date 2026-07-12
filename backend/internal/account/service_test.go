package account

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

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

func TestTOTPMatchesRFC6238SixDigitProfile(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	code, err := totpCode(secret, 1)
	require.NoError(t, err)
	require.Equal(t, "287082", code)
	require.True(t, validateTOTP(secret, code, time.Unix(59, 0)))
	require.False(t, validateTOTP(secret, "287083", time.Unix(59, 0)))
}

func TestRecoveryCodesAreUniqueAndNormalized(t *testing.T) {
	codes, err := generateRecoveryCodes(10)
	require.NoError(t, err)
	require.Len(t, codes, 10)
	require.Len(t, mapFromStrings(codes), 10)
	require.Equal(t, "abcdef", normalizeRecoveryCode("ABC-DEF"))
}

// mapFromStrings creates a set for recovery-code uniqueness assertions.
func mapFromStrings(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
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

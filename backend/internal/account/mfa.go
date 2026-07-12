package account

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- RFC 6238 requires HMAC-SHA1 interoperability.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// totpStep is the standard RFC 6238 time step.
	totpStep = 30 * time.Second
	// totpDigits is the broadly interoperable authenticator code length.
	totpDigits = 6
)

// generateTOTPSecret creates a 160-bit Base32 authenticator secret.
func generateTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

// totpURI returns an authenticator-compatible provisioning URI.
func totpURI(secret, issuer, accountName string) string {
	label := url.PathEscape(strings.TrimSpace(issuer) + ":" + strings.TrimSpace(accountName))
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", strings.TrimSpace(issuer))
	query.Set("algorithm", "SHA1")
	query.Set("digits", strconv.Itoa(totpDigits))
	query.Set("period", strconv.Itoa(int(totpStep.Seconds())))
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// validateTOTP accepts the current time window and one adjacent window for clock skew.
func validateTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	counter := now.Unix() / int64(totpStep.Seconds())
	for offset := int64(-1); offset <= 1; offset++ {
		candidate, err := totpCode(secret, uint64(counter+offset))
		if err == nil && subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// totpCode calculates one RFC 4226 dynamic truncation value.
func totpCode(secret string, counter uint64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, counter)
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000), nil
}

// generateRecoveryCodes creates printable single-use backup factors.
func generateRecoveryCodes(count int) ([]string, error) {
	values := make([]string, 0, count)
	for range count {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		values = append(values, strings.ToLower(encoded[:6]+"-"+encoded[6:]))
	}
	return values, nil
}

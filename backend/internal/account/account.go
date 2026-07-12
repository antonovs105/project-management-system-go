// Package account implements account recovery, email verification, session
// inventory, MFA, durable security events, and transactional email delivery.
package account

import "time"

const (
	// TokenPurposeVerifyEmail identifies local email-ownership challenges.
	TokenPurposeVerifyEmail = "verify_email"
	// TokenPurposeResetPassword identifies password recovery challenges.
	TokenPurposeResetPassword = "reset_password"
)

// ClientInfo captures the security-relevant request context stored with sessions and events.
type ClientInfo struct {
	IPAddress string
	UserAgent string
}

// Session is a user-visible authenticated browser session.
type Session struct {
	ID         string     `db:"id" json:"id"`
	UserAgent  string     `db:"user_agent" json:"user_agent"`
	IPAddress  string     `db:"ip_address" json:"ip_address"`
	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	LastSeenAt time.Time  `db:"last_seen_at" json:"last_seen_at"`
	ExpiresAt  time.Time  `db:"expires_at" json:"expires_at"`
	RevokedAt  *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	Current    bool       `json:"current"`
}

// SecurityEvent is a durable account-authentication event visible to its owner.
type SecurityEvent struct {
	ID          string         `db:"id" json:"id"`
	EventType   string         `db:"event_type" json:"event_type"`
	IPAddress   string         `db:"ip_address" json:"ip_address"`
	UserAgent   string         `db:"user_agent" json:"user_agent"`
	Metadata    map[string]any `db:"-" json:"metadata"`
	RawMetadata []byte         `db:"metadata" json:"-"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
}

// EmailMessage is one transactional email queued for delivery.
type EmailMessage struct {
	ID        string `db:"id"`
	Recipient string `db:"recipient"`
	Subject   string `db:"subject"`
	TextBody  string `db:"text_body"`
	Attempts  int    `db:"attempts"`
}

// MFAStatus reports whether a second factor is active for the current account.
type MFAStatus struct {
	Enabled bool `json:"enabled"`
}

// MFASetup contains the authenticator secret and provisioning URI shown once before confirmation.
type MFASetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

// MFARecoveryCodes contains one-time codes shown only after successful enrollment.
type MFARecoveryCodes struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

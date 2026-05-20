package remoteactor

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidResource      = errors.New("invalid remote actor resource")
	ErrNotFound             = errors.New("remote actor not found")
	ErrInvalidWebFinger     = errors.New("invalid webfinger response")
	ErrInvalidActorDocument = errors.New("invalid actor document")
	ErrLocalActorConflict   = errors.New("remote actor conflicts with local actor")
)

type Actor struct {
	ID                string          `db:"id" json:"id"`
	APID              string          `db:"ap_id" json:"ap_id"`
	Type              string          `db:"type" json:"type"`
	PreferredUsername string          `db:"preferred_username" json:"preferred_username"`
	Handle            string          `db:"handle" json:"handle"`
	Name              string          `db:"name" json:"name"`
	Summary           string          `db:"summary" json:"summary"`
	InboxURL          string          `db:"inbox_url" json:"inbox_url"`
	OutboxURL         string          `db:"outbox_url" json:"outbox_url"`
	FollowersURL      *string         `db:"followers_url" json:"followers_url,omitempty"`
	FollowingURL      *string         `db:"following_url" json:"following_url,omitempty"`
	PublicKeyID       string          `db:"public_key_id" json:"public_key_id"`
	PublicKeyPEM      string          `db:"public_key_pem" json:"-"`
	Document          json.RawMessage `db:"document" json:"-"`
	LastFetchedAt     *time.Time      `db:"last_fetched_at" json:"last_fetched_at,omitempty"`
	FetchError        *string         `db:"fetch_error" json:"fetch_error,omitempty"`
	FetchErrorAt      *time.Time      `db:"fetch_error_at" json:"fetch_error_at,omitempty"`
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time       `db:"updated_at" json:"updated_at"`
}

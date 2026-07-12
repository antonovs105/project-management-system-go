// Package outboundwebhook provides signed, retried project automation callbacks.
package outboundwebhook

import (
	"encoding/json"
	"time"
)

// SupportedEvents lists stable project activity event names accepted by webhooks.
var SupportedEvents = []string{
	"project.created", "project.updated", "project.archived", "project.restored",
	"ticket.created", "ticket.updated", "ticket.archived", "ticket.restored",
}

// Webhook is a project-scoped outbound callback without its stored secret.
type Webhook struct {
	ID        string    `db:"id" json:"id"`
	ProjectID string    `db:"project_id" json:"project_id"`
	CreatedBy string    `db:"created_by" json:"created_by"`
	Name      string    `db:"name" json:"name"`
	TargetURL string    `db:"target_url" json:"target_url"`
	Events    []string  `db:"events" json:"events"`
	Active    bool      `db:"active" json:"active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CreatedWebhook returns the signing secret exactly once.
type CreatedWebhook struct {
	Webhook
	Secret string `json:"secret"`
}

// Delivery is one durable event delivery attempt lifecycle.
type Delivery struct {
	ID             string          `db:"id" json:"id"`
	WebhookID      string          `db:"webhook_id" json:"webhook_id"`
	WebhookName    string          `db:"webhook_name" json:"webhook_name"`
	TargetURL      string          `db:"target_url" json:"target_url"`
	EventType      string          `db:"event_type" json:"event_type"`
	Payload        json.RawMessage `db:"payload" json:"payload"`
	Status         string          `db:"status" json:"status"`
	Attempts       int             `db:"attempts" json:"attempts"`
	MaxAttempts    int             `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt  time.Time       `db:"next_attempt_at" json:"next_attempt_at"`
	LastError      string          `db:"last_error" json:"last_error"`
	LastStatusCode *int            `db:"last_status_code" json:"last_status_code"`
	DeliveredAt    *time.Time      `db:"delivered_at" json:"delivered_at"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
	SecretCipher   string          `db:"secret_ciphertext" json:"-"`
}

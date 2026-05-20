package project

import "time"

type Project struct {
	ID            string    `db:"id" json:"id"`
	APID          string    `db:"ap_id" json:"ap_id"`
	Name          string    `db:"name" json:"name"`
	Description   string    `db:"description" json:"description"`
	OwnerID       string    `db:"owner_id" json:"owner_id"`
	Handle        string    `db:"handle" json:"handle"`
	PublicKeyPEM  string    `db:"public_key_pem" json:"-"`
	PrivateKeyPEM string    `db:"private_key_pem" json:"-"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

type ProjectInvite struct {
	ID             string    `db:"id" json:"id"`
	APID           string    `db:"ap_id" json:"ap_id"`
	ProjectID      string    `db:"project_id" json:"project_id"`
	InviterActorID string    `db:"inviter_actor_id" json:"inviter_actor_id"`
	InviteeActorID string    `db:"invitee_actor_id" json:"invitee_actor_id"`
	Role           string    `db:"role" json:"role"`
	Status         string    `db:"status" json:"status"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

package httpsig

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Repository loads signing and verification keys for ActivityPub actors.
type Repository interface {
	ActivePrivateKey(ctx context.Context, actorID string) (*ActorKey, error)
	PublicKeyByKeyID(ctx context.Context, keyID string) (*ActorKey, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed HTTP signature repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// ActivePrivateKey returns the active local private key for an actor.
func (r *PgRepository) ActivePrivateKey(ctx context.Context, actorID string) (*ActorKey, error) {
	var key ActorKey
	if err := r.db.GetContext(ctx, &key, `
		SELECT
			k.actor_id::text,
			a.ap_id AS actor_ap_id,
			key_id,
			algorithm,
			public_key_pem,
			private_key_pem
		FROM actor_keys k
		JOIN actors a ON a.id = k.actor_id
		WHERE k.actor_id = $1 AND active = true AND private_key_pem IS NOT NULL
		ORDER BY k.created_at DESC
		LIMIT 1
	`, actorID); err != nil {
		return nil, err
	}
	return &key, nil
}

// PublicKeyByKeyID returns the active public key for a key ID.
func (r *PgRepository) PublicKeyByKeyID(ctx context.Context, keyID string) (*ActorKey, error) {
	var key ActorKey
	if err := r.db.GetContext(ctx, &key, `
		SELECT
			k.actor_id::text,
			a.ap_id AS actor_ap_id,
			key_id,
			algorithm,
			public_key_pem,
			COALESCE(private_key_pem, '') AS private_key_pem
		FROM actor_keys k
		JOIN actors a ON a.id = k.actor_id
		WHERE key_id = $1 AND active = true
		LIMIT 1
	`, keyID); err != nil {
		return nil, err
	}
	return &key, nil
}

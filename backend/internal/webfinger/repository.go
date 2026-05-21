package webfinger

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Repository loads local actors that can be exposed through WebFinger.
type Repository interface {
	FindLocalActor(ctx context.Context, preferredUsername string) (*ActorResource, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed WebFinger repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// FindLocalActor returns a local actor by preferred username.
func (r *PgRepository) FindLocalActor(ctx context.Context, preferredUsername string) (*ActorResource, error) {
	var actor ActorResource
	err := r.db.GetContext(ctx, &actor, `
		SELECT a.preferred_username AS username, a.handle, a.ap_id
		FROM actors a
		WHERE lower(a.preferred_username) = lower($1)
			AND a.is_local = true
			AND a.type IN ('Person', 'Group')
	`, preferredUsername)
	if err != nil {
		return nil, err
	}
	return &actor, nil
}

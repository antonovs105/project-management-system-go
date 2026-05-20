package webfinger

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	FindLocalActor(ctx context.Context, preferredUsername string) (*ActorResource, error)
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

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

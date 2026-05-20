package webfinger

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	FindLocalUserActor(ctx context.Context, username string) (*ActorResource, error)
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) FindLocalUserActor(ctx context.Context, username string) (*ActorResource, error) {
	var actor ActorResource
	err := r.db.GetContext(ctx, &actor, `
		SELECT u.username, a.handle, a.ap_id
		FROM users u
		JOIN actors a ON a.id = u.id
		WHERE lower(u.username) = lower($1)
			AND a.is_local = true
			AND a.type = 'Person'
	`, username)
	if err != nil {
		return nil, err
	}
	return &actor, nil
}

package remoteactor

import (
	"context"
	"database/sql"
	"errors"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	RemoteActorByAPID(ctx context.Context, apID string) (*Actor, error)
	UpsertRemoteActor(ctx context.Context, actor *Actor) error
	RecordRemoteActorFetchFailure(ctx context.Context, apID string, fetchError string) error
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) RemoteActorByAPID(ctx context.Context, apID string) (*Actor, error) {
	var actor Actor
	err := r.db.GetContext(ctx, &actor, `
		SELECT
			actor.id::text,
			actor.ap_id,
			actor.type,
			actor.preferred_username,
			actor.handle,
			actor.name,
			actor.summary,
			actor.inbox_url,
			actor.outbox_url,
			actor.followers_url,
			actor.following_url,
			COALESCE(key.key_id, '') AS public_key_id,
			COALESCE(key.public_key_pem, '') AS public_key_pem,
			COALESCE(object.document, '{}'::jsonb) AS document,
			actor.last_fetched_at,
			actor.fetch_error,
			actor.created_at,
			actor.updated_at
		FROM actors actor
		LEFT JOIN actor_keys key ON key.actor_id = actor.id AND key.active = true
		LEFT JOIN ap_objects object ON object.ap_id = actor.ap_id
		WHERE actor.ap_id = $1 AND actor.is_local = false
	`, apID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &actor, nil
}

func (r *PgRepository) UpsertRemoteActor(ctx context.Context, actor *Actor) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRowxContext(ctx, `
		INSERT INTO actors (
			ap_id, type, preferred_username, handle, name, summary,
			inbox_url, outbox_url, followers_url, following_url,
			is_local, last_fetched_at, fetch_error
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, false, now(), NULL)
		ON CONFLICT (ap_id) DO UPDATE
		SET
			type = EXCLUDED.type,
			preferred_username = EXCLUDED.preferred_username,
			handle = EXCLUDED.handle,
			name = EXCLUDED.name,
			summary = EXCLUDED.summary,
			inbox_url = EXCLUDED.inbox_url,
			outbox_url = EXCLUDED.outbox_url,
			followers_url = EXCLUDED.followers_url,
			following_url = EXCLUDED.following_url,
			last_fetched_at = now(),
			fetch_error = NULL
		WHERE actors.is_local = false
		RETURNING id::text, last_fetched_at, fetch_error, created_at, updated_at
	`,
		actor.APID,
		actor.Type,
		actor.PreferredUsername,
		actor.Handle,
		actor.Name,
		actor.Summary,
		actor.InboxURL,
		actor.OutboxURL,
		actor.FollowersURL,
		actor.FollowingURL,
	).Scan(&actor.ID, &actor.LastFetchedAt, &actor.FetchError, &actor.CreatedAt, &actor.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLocalActorConflict
		}
		return err
	}

	if actor.PublicKeyID != "" && actor.PublicKeyPEM != "" {
		var existingActorID string
		if err := tx.GetContext(ctx, &existingActorID, `
			SELECT actor_id::text
			FROM actor_keys
			WHERE key_id = $1
		`, actor.PublicKeyID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if existingActorID != "" && existingActorID != actor.ID {
			return ErrLocalActorConflict
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE actor_keys
			SET active = false
			WHERE actor_id = $1 AND key_id <> $2 AND active = true
		`, actor.ID, actor.PublicKeyID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actor_keys (
				actor_id, key_id, algorithm, public_key_pem, private_key_pem, active
			)
			VALUES ($1, $2, $3, $4, NULL, true)
			ON CONFLICT (key_id) DO UPDATE
			SET
				actor_id = EXCLUDED.actor_id,
				algorithm = EXCLUDED.algorithm,
				public_key_pem = EXCLUDED.public_key_pem,
				private_key_pem = NULL,
				active = true
		`, actor.ID, actor.PublicKeyID, httpsig.AlgorithmRSAV15SHA256, actor.PublicKeyPEM); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, $2, $3, NULL, NULL, $4)
		ON CONFLICT (ap_id) DO UPDATE
		SET
			object_type = EXCLUDED.object_type,
			actor_id = EXCLUDED.actor_id,
			local_ref_table = NULL,
			local_ref_id = NULL,
			document = EXCLUDED.document,
			is_deleted = false
	`, actor.APID, actor.Type, actor.ID, actor.Document); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) RecordRemoteActorFetchFailure(ctx context.Context, apID string, fetchError string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE actors
		SET fetch_error = $2
		WHERE ap_id = $1 AND is_local = false
	`, apID, fetchError)
	return err
}

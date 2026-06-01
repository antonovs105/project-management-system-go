package federation

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

// NewRepository creates a PostgreSQL-backed personal federation repository.
func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

// LocalUserActor returns the authenticated local user actor.
func (r *PgRepository) LocalUserActor(ctx context.Context, userID string) (*LocalActor, error) {
	var actor LocalActor
	err := r.db.GetContext(ctx, &actor, `
		SELECT actor.id::text, actor.ap_id
		FROM users
		JOIN actors actor ON actor.id = users.id
		WHERE users.id = $1 AND actor.is_local = true
	`, userID)
	return &actor, err
}

// ListInboxActivities returns normalized ActivityPub inbox items for a local actor.
func (r *PgRepository) ListInboxActivities(ctx context.Context, userID string, options ListOptions) ([]InboxActivity, error) {
	activities := make([]InboxActivity, 0)
	if err := r.db.SelectContext(ctx, &activities, `
		SELECT
			activity.id::text AS id,
			activity.ap_id AS activity_ap_id,
			activity.activity_type,
			actor.id::text AS actor_id,
			actor.ap_id AS actor_ap_id,
			actor.type AS actor_type,
			actor.handle AS actor_handle,
			actor.name AS actor_name,
			activity.object_ap_id,
			CASE
				WHEN jsonb_typeof(activity.document->'object') = 'object'
					AND jsonb_typeof(activity.document->'object'->'type') = 'array'
					THEN activity.document->'object'->'type'->>0
				WHEN jsonb_typeof(activity.document->'object') = 'object'
					THEN NULLIF(activity.document #>> '{object,type}', '')
				ELSE NULL
			END AS object_type,
			CASE
				WHEN jsonb_typeof(activity.document->'object') = 'object'
					THEN NULLIF(activity.document #>> '{object,name}', '')
				ELSE NULL
			END AS object_name,
			CASE
				WHEN jsonb_typeof(activity.document->'object') = 'object'
					THEN NULLIF(activity.document #>> '{object,content}', '')
				ELSE NULL
			END AS object_content,
			activity.target_ap_id,
			target_actor.id::text AS target_actor_id,
			target_actor.type AS target_type,
			target_actor.handle AS target_handle,
			target_actor.name AS target_name,
			inbox_item.received_at,
			activity.created_at
		FROM actor_inbox_items inbox_item
		JOIN ap_activities activity ON activity.id = inbox_item.activity_id
		JOIN actors actor ON actor.id = activity.actor_id
		LEFT JOIN actors target_actor ON target_actor.ap_id = activity.target_ap_id
		WHERE inbox_item.actor_id = $1
		ORDER BY inbox_item.received_at DESC, activity.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return activities, nil
}

// ListRemoteFollows returns remote actors followed by a local actor.
func (r *PgRepository) ListRemoteFollows(ctx context.Context, userID string, options ListOptions) ([]RemoteFollow, error) {
	follows := make([]RemoteFollow, 0)
	if err := r.db.SelectContext(ctx, &follows, `
		SELECT
			followed.id::text AS actor_id,
			followed.ap_id AS actor_ap_id,
			followed.type AS actor_type,
			followed.preferred_username,
			followed.handle,
			followed.name,
			followed.summary,
			followed.inbox_url,
			followed.outbox_url,
			followed.followers_url,
			followed.following_url,
			follow.state,
			follow.created_at,
			follow.updated_at
		FROM actor_follows follow
		JOIN actors followed ON followed.id = follow.followed_actor_id
		WHERE follow.follower_actor_id = $1
			AND followed.is_local = false
			AND ($2 = '' OR follow.state = $2)
		ORDER BY follow.updated_at DESC, follow.created_at DESC, followed.ap_id ASC
		LIMIT $3 OFFSET $4
	`, userID, options.State, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return follows, nil
}

// StoreOutgoingFollow stores a local Follow activity and pending follow relation.
func (r *PgRepository) StoreOutgoingFollow(ctx context.Context, actorID, remoteActorID, activityID, activityAPID, remoteActorAPID string, document []byte) (*RemoteFollow, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()

	var existingState string
	err = tx.GetContext(ctx, &existingState, `
		SELECT state
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
		FOR UPDATE
	`, actorID, remoteActorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	if existingState == "pending" || existingState == "accepted" {
		follow, err := loadRemoteFollowTx(ctx, tx, actorID, remoteActorID)
		if err != nil {
			return nil, false, err
		}
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return follow, false, nil
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, document)
		VALUES ($1, $2, 'Follow', $3, $4, $5)
	`, activityID, activityAPID, actorID, remoteActorAPID, document); err != nil {
		return nil, false, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return nil, false, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'pending')
		ON CONFLICT (follower_actor_id, followed_actor_id)
		DO UPDATE SET state = 'pending'
	`, actorID, remoteActorID); err != nil {
		return nil, false, err
	}

	follow, err := loadRemoteFollowTx(ctx, tx, actorID, remoteActorID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return follow, true, nil
}

// loadRemoteFollowTx loads one followed remote actor projection.
func loadRemoteFollowTx(ctx context.Context, tx *sqlx.Tx, actorID, remoteActorID string) (*RemoteFollow, error) {
	var follow RemoteFollow
	err := tx.GetContext(ctx, &follow, `
		SELECT
			followed.id::text AS actor_id,
			followed.ap_id AS actor_ap_id,
			followed.type AS actor_type,
			followed.preferred_username,
			followed.handle,
			followed.name,
			followed.summary,
			followed.inbox_url,
			followed.outbox_url,
			followed.followers_url,
			followed.following_url,
			follow.state,
			follow.created_at,
			follow.updated_at
		FROM actor_follows follow
		JOIN actors followed ON followed.id = follow.followed_actor_id
		WHERE follow.follower_actor_id = $1
			AND follow.followed_actor_id = $2
			AND followed.is_local = false
	`, actorID, remoteActorID)
	if err != nil {
		return nil, err
	}
	return &follow, nil
}

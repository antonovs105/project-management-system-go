package federation

import (
	"context"

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

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

// ListRemoteProjectInvites returns remote project invites addressed to a local actor.
func (r *PgRepository) ListRemoteProjectInvites(ctx context.Context, userID string, options ListOptions) ([]RemoteProjectInvite, error) {
	invites := make([]RemoteProjectInvite, 0)
	if err := r.db.SelectContext(ctx, &invites, remoteProjectInviteSelect()+`
		WHERE invite.invitee_actor_id = $1
			AND ($2 = '' OR invite.status = $2)
		ORDER BY invite.updated_at DESC, invite.created_at DESC
		LIMIT $3 OFFSET $4
	`, userID, options.State, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return invites, nil
}

// ListRemoteProjects returns accepted remote project workspaces for a local actor.
func (r *PgRepository) ListRemoteProjects(ctx context.Context, userID string, options ListOptions) ([]RemoteProject, error) {
	projects := make([]RemoteProject, 0)
	if err := r.db.SelectContext(ctx, &projects, remoteProjectSelect()+`
		WHERE invite.invitee_actor_id = $1
			AND invite.status = 'accepted'
		ORDER BY COALESCE(response.created_at, invite.updated_at) DESC, invite.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return projects, nil
}

// GetRemoteProject loads one accepted remote project workspace for a local actor.
func (r *PgRepository) GetRemoteProject(ctx context.Context, userID string, projectID string) (*RemoteProject, error) {
	var project RemoteProject
	err := r.db.GetContext(ctx, &project, remoteProjectSelect()+`
		WHERE invite.id = $1
			AND invite.invitee_actor_id = $2
			AND invite.status = 'accepted'
	`, projectID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRemoteProjectNotFound
		}
		return nil, err
	}
	return &project, nil
}

// GetRemoteProjectInvite loads one remote project invite for the current user.
func (r *PgRepository) GetRemoteProjectInvite(ctx context.Context, userID string, inviteID string) (*RemoteProjectInvite, error) {
	var invite RemoteProjectInvite
	err := r.db.GetContext(ctx, &invite, remoteProjectInviteSelect()+`
		WHERE invite.id = $1
			AND invite.invitee_actor_id = $2
	`, inviteID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRemoteInviteNotFound
		}
		return nil, err
	}
	return &invite, nil
}

// AcceptRemoteProjectInvite accepts a pending remote project invite.
func (r *PgRepository) AcceptRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error) {
	return r.respondRemoteProjectInvite(ctx, userID, inviteID, "Accept", "accepted", activityID, activityAPID, document)
}

// RejectRemoteProjectInvite rejects a pending remote project invite.
func (r *PgRepository) RejectRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error) {
	return r.respondRemoteProjectInvite(ctx, userID, inviteID, "Reject", "rejected", activityID, activityAPID, document)
}

// respondRemoteProjectInvite stores a local Accept or Reject response for a remote invite.
func (r *PgRepository) respondRemoteProjectInvite(ctx context.Context, userID string, inviteID string, activityType string, status string, activityID string, activityAPID string, document []byte) (*RemoteInviteResponse, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invite, err := loadRemoteProjectInviteForUpdate(ctx, tx, inviteID, userID)
	if err != nil {
		return nil, err
	}
	if invite.Status != "pending" {
		return nil, ErrRemoteInviteNotPending
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, activityID, activityAPID, activityType, userID, invite.InviteAPID, invite.ProjectAPID, document); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, userID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if status == "accepted" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
			SELECT $1, actor.id, 'accepted'
			FROM actors actor
			WHERE actor.ap_id = $2
				AND actor.is_local = false
			ON CONFLICT (follower_actor_id, followed_actor_id)
			DO UPDATE SET state = 'accepted'
		`, userID, invite.ProjectAPID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE remote_project_invites
		SET status = $2,
			response_activity_id = $3
		WHERE id = $1
	`, invite.ID, status, activityID); err != nil {
		return nil, err
	}

	updated, err := loadRemoteProjectInvite(ctx, tx, invite.ID, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RemoteInviteResponse{
		Invite:         updated,
		ActivityID:     activityID,
		TargetInboxURL: invite.TargetInboxURL,
	}, nil
}

// StoreRemoteProjectActivity stores an outbound ActivityPub activity for a remote project.
func (r *PgRepository) StoreRemoteProjectActivity(ctx context.Context, userID string, projectID string, activityID string, activityAPID string, activityType string, objectAPID string, targetAPID *string, document []byte) (*RemoteProjectActivity, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	project, err := loadRemoteProjectForUpdate(ctx, tx, projectID, userID)
	if err != nil {
		return nil, err
	}
	var target any
	if targetAPID != nil {
		target = *targetAPID
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, activityID, activityAPID, activityType, userID, objectAPID, target, document); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, userID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &RemoteProjectActivity{ActivityID: activityID, TargetInboxURL: project.TargetInboxURL}, nil
}

// getContexter is the subset shared by sqlx.DB and sqlx.Tx for one-row loads.
type getContexter interface {
	GetContext(ctx context.Context, dest any, query string, args ...any) error
}

// remoteProjectInviteSelect returns the shared remote invite projection query.
func remoteProjectInviteSelect() string {
	return `
		SELECT
			invite.id::text,
			invite.invite_ap_id,
			invite.activity_id::text,
			invite.project_ap_id,
			invite.project_name,
			invite.inviter_actor_id::text,
			inviter.ap_id AS inviter_ap_id,
			inviter.handle AS inviter_handle,
			inviter.name AS inviter_name,
			invite.invitee_actor_id::text,
			invite.role,
			invite.target_inbox_url,
			invite.status,
			invite.created_at,
			invite.updated_at,
			response.created_at AS resolved_at
		FROM remote_project_invites invite
		JOIN actors inviter ON inviter.id = invite.inviter_actor_id
		LEFT JOIN ap_activities response ON response.id = invite.response_activity_id
	`
}

// remoteProjectSelect returns the shared accepted remote project query.
func remoteProjectSelect() string {
	return `
		SELECT
			invite.id::text,
			invite.project_ap_id,
			invite.project_name,
			invite.role,
			invite.target_inbox_url,
			invite.inviter_actor_id::text,
			inviter.ap_id AS inviter_ap_id,
			inviter.handle AS inviter_handle,
			inviter.name AS inviter_name,
			remote_project.id::text AS remote_actor_id,
			remote_project.handle AS remote_handle,
			invite.created_at,
			invite.updated_at,
			response.created_at AS resolved_at
		FROM remote_project_invites invite
		JOIN actors inviter ON inviter.id = invite.inviter_actor_id
		LEFT JOIN ap_activities response ON response.id = invite.response_activity_id
		LEFT JOIN actors remote_project
			ON remote_project.ap_id = invite.project_ap_id
			AND remote_project.is_local = false
	`
}

// loadRemoteProjectInviteForUpdate loads and locks one remote invite for mutation.
func loadRemoteProjectInviteForUpdate(ctx context.Context, tx *sqlx.Tx, inviteID string, userID string) (*RemoteProjectInvite, error) {
	var invite RemoteProjectInvite
	err := tx.GetContext(ctx, &invite, remoteProjectInviteSelect()+`
		WHERE invite.id = $1
			AND invite.invitee_actor_id = $2
		FOR UPDATE OF invite
	`, inviteID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRemoteInviteNotFound
		}
		return nil, err
	}
	return &invite, nil
}

// loadRemoteProjectForUpdate locks one accepted remote project invite for outbound work.
func loadRemoteProjectForUpdate(ctx context.Context, tx *sqlx.Tx, projectID string, userID string) (*RemoteProject, error) {
	var project RemoteProject
	err := tx.GetContext(ctx, &project, remoteProjectSelect()+`
		WHERE invite.id = $1
			AND invite.invitee_actor_id = $2
			AND invite.status = 'accepted'
		FOR UPDATE OF invite
	`, projectID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRemoteProjectNotFound
		}
		return nil, err
	}
	return &project, nil
}

// loadRemoteProjectInvite loads one remote invite projection without locking.
func loadRemoteProjectInvite(ctx context.Context, q getContexter, inviteID string, userID string) (*RemoteProjectInvite, error) {
	var invite RemoteProjectInvite
	err := q.GetContext(ctx, &invite, remoteProjectInviteSelect()+`
		WHERE invite.id = $1
			AND invite.invitee_actor_id = $2
	`, inviteID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRemoteInviteNotFound
		}
		return nil, err
	}
	return &invite, nil
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

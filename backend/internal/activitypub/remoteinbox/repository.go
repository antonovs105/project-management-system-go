package remoteinbox

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	FindLocalActorIDByAPID(ctx context.Context, apID string) (string, error)
	FindActorAPIDByID(ctx context.Context, actorID string) (string, error)
	StoreInboundActivity(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
}

type PgRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) FindLocalActorIDByAPID(ctx context.Context, apID string) (string, error) {
	var id string
	err := r.db.GetContext(ctx, &id, `
		SELECT id::text
		FROM actors
		WHERE ap_id = $1 AND is_local = true
	`, apID)
	return id, err
}

func (r *PgRepository) FindActorAPIDByID(ctx context.Context, actorID string) (string, error) {
	var apID string
	err := r.db.GetContext(ctx, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

func (r *PgRepository) StoreInboundActivity(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var activityID string
	err = tx.QueryRowxContext(ctx, `
		INSERT INTO ap_activities (
			ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (ap_id) DO NOTHING
		RETURNING id::text
	`,
		activity.ID,
		activity.Type,
		activity.ActorID,
		activity.ObjectAPID,
		activity.TargetAPID,
		activity.Document,
	).Scan(&activityID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	duplicateActivity := false
	if activityID == "" {
		duplicateActivity = true

		var storedActorID string
		if err := tx.QueryRowxContext(ctx, `
			SELECT id::text, actor_id::text
			FROM ap_activities
			WHERE ap_id = $1
		`, activity.ID).Scan(&activityID, &storedActorID); err != nil {
			return nil, err
		}
		if storedActorID != activity.ActorID {
			return nil, ErrActivityConflict
		}
	}

	var receivedAt sql.NullTime
	err = tx.QueryRowxContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (actor_id, activity_ap_id) DO NOTHING
		RETURNING received_at
	`, targetActorID, activityID, activity.ID).Scan(&receivedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	duplicateInboxItem := false
	if !receivedAt.Valid {
		duplicateInboxItem = true
		if err := tx.QueryRowxContext(ctx, `
			SELECT received_at
			FROM actor_inbox_items
			WHERE actor_id = $1 AND activity_ap_id = $2
		`, targetActorID, activity.ID).Scan(&receivedAt); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &AcceptedActivity{
		ActivityID:   activityID,
		ActivityAPID: activity.ID,
		ReceivedAt:   receivedAt.Time,
		Duplicate:    duplicateActivity || duplicateInboxItem,
	}, nil
}

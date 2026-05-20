package remoteinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type Repository interface {
	FindLocalActorIDByAPID(ctx context.Context, apID string) (string, error)
	FindActorAPIDByID(ctx context.Context, actorID string) (string, error)
	IsDomainBlocked(ctx context.Context, domains []string) (bool, error)
	IsProjectActor(ctx context.Context, actorID string) (bool, error)
	RemoteProjectFollowerInboxesExceptActor(ctx context.Context, projectID string, actorID string) ([]string, error)
	StoreInboundActivity(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundCreateNote(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundCreateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundUpdateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundAddTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundRemoveTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundDeleteTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundAcceptInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	StoreInboundRejectInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error)
	AcceptProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) (*FollowResponse, error)
	UndoProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) error
}

type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

func NewRepository(db *sqlx.DB, configs ...activitypub.Config) Repository {
	cfg := activitypub.NewConfig("", "")
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return &PgRepository{db: db, cfg: cfg}
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

func (r *PgRepository) IsDomainBlocked(ctx context.Context, domains []string) (bool, error) {
	if len(domains) == 0 {
		return false, nil
	}
	var blocked bool
	err := r.db.GetContext(ctx, &blocked, `
		SELECT EXISTS(
			SELECT 1
			FROM federation_domain_blocks
			WHERE domain = ANY($1)
		)
	`, pq.Array(domains))
	return blocked, err
}

func (r *PgRepository) IsProjectActor(ctx context.Context, actorID string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, actorID)
	return exists, err
}

func (r *PgRepository) RemoteProjectFollowerInboxesExceptActor(ctx context.Context, projectID string, actorID string) ([]string, error) {
	var inboxes []string
	err := r.db.SelectContext(ctx, &inboxes, `
		WITH sender AS (
			SELECT inbox_url
			FROM actors
			WHERE id = $2
		)
		SELECT DISTINCT follower.inbox_url
		FROM actor_follows follow
		JOIN actors follower ON follower.id = follow.follower_actor_id
		LEFT JOIN sender ON true
		WHERE follow.followed_actor_id = $1
			AND follow.state = 'accepted'
			AND follower.is_local = false
			AND follower.inbox_url <> ''
			AND follower.id <> $2
			AND (sender.inbox_url IS NULL OR follower.inbox_url <> sender.inbox_url)
		ORDER BY follower.inbox_url ASC
	`, projectID, actorID)
	return inboxes, err
}

func (r *PgRepository) StoreInboundActivity(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundCreateNote(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectNote == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.insertRemoteNoteCommentTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundCreateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectTicket == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.insertRemoteTicketTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundUpdateTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectTicket == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.updateRemoteTicketTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundAddTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectAPID == nil || activity.TargetAPID == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.insertRemoteTicketAssigneeTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundRemoveTicketAssignee(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectAPID == nil || activity.TargetAPID == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.deleteRemoteTicketAssigneeTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundDeleteTicket(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	if activity.ObjectAPID == nil {
		return nil, ErrInvalidActivity
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.deleteRemoteTicketTx(ctx, tx, targetActorID, activity); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) StoreInboundAcceptInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	return r.storeInboundInviteResponse(ctx, targetActorID, activity, InviteResponseAccept)
}

func (r *PgRepository) StoreInboundRejectInvite(ctx context.Context, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	return r.storeInboundInviteResponse(ctx, targetActorID, activity, InviteResponseReject)
}

func (r *PgRepository) storeInboundInviteResponse(ctx context.Context, targetActorID string, activity *InboundActivity, response InviteResponseType) (*AcceptedActivity, error) {
	if activity.ObjectAPID == nil {
		return nil, ErrInvalidActivity
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	accepted, err := r.storeInboundActivityTx(ctx, tx, targetActorID, activity)
	if err != nil {
		return nil, err
	}
	if !accepted.Duplicate {
		if err := r.applyInboundInviteResponseTx(ctx, tx, targetActorID, activity, accepted.ActivityID, response); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return accepted, nil
}

func (r *PgRepository) applyInboundInviteResponseTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity, responseActivityID string, response InviteResponseType) error {
	var invite struct {
		ID             string `db:"id"`
		ProjectID      string `db:"project_id"`
		InviteeActorID string `db:"invitee_actor_id"`
		Status         string `db:"status"`
	}
	if err := tx.GetContext(ctx, &invite, `
		SELECT
			id::text,
			project_id::text,
			invitee_actor_id::text,
			status
		FROM project_invites
		WHERE ap_id = $1
		FOR UPDATE
	`, *activity.ObjectAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}
	if invite.ProjectID != targetActorID {
		return ErrInvalidActivity
	}
	if invite.InviteeActorID != activity.ActorID {
		return ErrForbiddenActor
	}
	if invite.Status != "pending" {
		return ErrInvalidActivity
	}

	switch response {
	case InviteResponseAccept:
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
			VALUES ($1, $2, 'accepted')
			ON CONFLICT (follower_actor_id, followed_actor_id)
			DO UPDATE SET state = 'accepted'
		`, activity.ActorID, targetActorID); err != nil {
			return err
		}
	case InviteResponseReject:
	default:
		return ErrInvalidActivity
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE project_invites
		SET status = $2,
			response_activity_id = $3
		WHERE id = $1
	`, invite.ID, string(response), responseActivityID)
	return err
}

func (r *PgRepository) storeInboundActivityTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) (*AcceptedActivity, error) {
	var activityID string
	err := tx.QueryRowxContext(ctx, `
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

	return &AcceptedActivity{
		ActivityID:   activityID,
		ActivityAPID: activity.ID,
		ReceivedAt:   receivedAt.Time,
		Duplicate:    duplicateActivity || duplicateInboxItem,
	}, nil
}

func (r *PgRepository) insertRemoteNoteCommentTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	note := activity.ObjectNote

	var ticketID string
	if err := tx.GetContext(ctx, &ticketID, `
		SELECT ticket.id::text
		FROM tickets ticket
		JOIN projects project ON project.id = ticket.project_id
		WHERE project.id = $1 AND ticket.ap_id = $2
	`, targetActorID, note.InReplyTo); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	commentID, err := activitypub.NewID()
	if err != nil {
		return err
	}

	var storedCommentID string
	if err := tx.GetContext(ctx, &storedCommentID, `
		WITH inserted AS (
			INSERT INTO comments (id, ap_id, ticket_id, author_id, content)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (ap_id) DO NOTHING
			RETURNING id::text
		)
		SELECT id FROM inserted
		UNION ALL
		SELECT id::text FROM comments WHERE ap_id = $2
		LIMIT 1
	`, commentID, note.ID, ticketID, activity.ActorID, note.Content); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Note', $2, 'comments', $3, $4)
		ON CONFLICT (ap_id) DO UPDATE
		SET object_type = EXCLUDED.object_type,
			actor_id = EXCLUDED.actor_id,
			local_ref_table = EXCLUDED.local_ref_table,
			local_ref_id = EXCLUDED.local_ref_id,
			document = EXCLUDED.document,
			is_deleted = false
	`, note.ID, activity.ActorID, storedCommentID, note.Document)
	return err
}

func (r *PgRepository) insertRemoteTicketTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	ticket := activity.ObjectTicket

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	priority, ok := normalizeTicketPriority(ticket.Priority)
	if !ok {
		return ErrInvalidActivity
	}
	ticketType, ok := normalizeTicketType(ticket.TicketType)
	if !ok {
		return ErrInvalidActivity
	}
	status := "open"
	if ticket.IsResolved {
		status = "done"
	}

	ticketID, err := activitypub.NewID()
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tickets (
			id, ap_id, project_id, reporter_id, title, description,
			status, priority, type, is_resolved, resolved_at, resolved_by_actor_id
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			CASE WHEN $10 THEN now() ELSE NULL END,
			CASE WHEN $10 THEN CAST($4 AS uuid) ELSE NULL END
		)
		ON CONFLICT (ap_id) DO NOTHING
	`, ticketID, ticket.ID, targetActorID, activity.ActorID, ticket.Name, ticket.Content, status, priority, ticketType, ticket.IsResolved); err != nil {
		return err
	}

	var stored struct {
		ID         string `db:"id"`
		ProjectID  string `db:"project_id"`
		ReporterID string `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &stored, `
		SELECT id::text, project_id::text, reporter_id::text
		FROM tickets
		WHERE ap_id = $1
	`, ticket.ID); err != nil {
		return err
	}
	if stored.ProjectID != targetActorID || stored.ReporterID != activity.ActorID {
		return ErrActivityConflict
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Ticket', $2, 'tickets', $3, $4)
		ON CONFLICT (ap_id) DO UPDATE
		SET object_type = EXCLUDED.object_type,
			actor_id = EXCLUDED.actor_id,
			local_ref_table = EXCLUDED.local_ref_table,
			local_ref_id = EXCLUDED.local_ref_id,
			document = EXCLUDED.document,
			is_deleted = false
	`, ticket.ID, activity.ActorID, stored.ID, ticket.Document)
	return err
}

func (r *PgRepository) updateRemoteTicketTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	ticket := activity.ObjectTicket

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	var stored struct {
		ID          string `db:"id"`
		ProjectID   string `db:"project_id"`
		ReporterID  string `db:"reporter_id"`
		Title       string `db:"title"`
		Description string `db:"description"`
		Status      string `db:"status"`
		Priority    string `db:"priority"`
		Type        string `db:"type"`
	}
	if err := tx.GetContext(ctx, &stored, `
		SELECT id::text, project_id::text, reporter_id::text, title, description, status, priority, type
		FROM tickets
		WHERE ap_id = $1
		FOR UPDATE
	`, ticket.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}
	if stored.ProjectID != targetActorID || stored.ReporterID != activity.ActorID {
		return ErrActivityConflict
	}

	title := stored.Title
	if ticket.HasName {
		title = ticket.Name
	}
	description := stored.Description
	if ticket.HasContent {
		description = ticket.Content
	}
	status := stored.Status
	if ticket.HasStatus {
		normalizedStatus, ok := normalizeTicketStatus(ticket.Status)
		if !ok {
			return ErrInvalidActivity
		}
		status = normalizedStatus
	}
	if ticket.HasIsResolved {
		if ticket.IsResolved {
			status = "done"
		} else if !ticket.HasStatus && status == "done" {
			status = "open"
		}
	}
	priority := stored.Priority
	if ticket.HasPriority {
		normalizedPriority, ok := normalizeTicketPriority(ticket.Priority)
		if !ok {
			return ErrInvalidActivity
		}
		priority = normalizedPriority
	}
	ticketType := stored.Type
	if ticket.HasTicketType {
		normalizedType, ok := normalizeTicketType(ticket.TicketType)
		if !ok {
			return ErrInvalidActivity
		}
		ticketType = normalizedType
	}
	isResolved := status == "done"

	if _, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET title = $2,
			description = $3,
			status = $4,
			priority = $5,
			type = $6,
			is_resolved = $7,
			resolved_at = CASE
				WHEN $7 AND resolved_at IS NULL THEN now()
				WHEN NOT $7 THEN NULL
				ELSE resolved_at
			END,
			resolved_by_actor_id = CASE
				WHEN $7 THEN CAST($8 AS uuid)
				ELSE NULL
			END
		WHERE id = $1
	`, stored.ID, title, description, status, priority, ticketType, isResolved, activity.ActorID); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Ticket', $2, 'tickets', $3, $4)
		ON CONFLICT (ap_id) DO UPDATE
		SET object_type = EXCLUDED.object_type,
			actor_id = EXCLUDED.actor_id,
			local_ref_table = EXCLUDED.local_ref_table,
			local_ref_id = EXCLUDED.local_ref_id,
			document = EXCLUDED.document,
			is_deleted = false
	`, ticket.ID, activity.ActorID, stored.ID, ticket.Document)
	return err
}

func (r *PgRepository) insertRemoteTicketAssigneeTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	assigneeAPID := *activity.ObjectAPID
	ticketAPID := *activity.TargetAPID

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	var storedTicket struct {
		ID         string `db:"id"`
		ProjectID  string `db:"project_id"`
		ReporterID string `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &storedTicket, `
		SELECT id::text, project_id::text, reporter_id::text
		FROM tickets
		WHERE ap_id = $1
		FOR UPDATE
	`, ticketAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}
	if storedTicket.ProjectID != targetActorID || storedTicket.ReporterID != activity.ActorID {
		return ErrActivityConflict
	}

	var assigneeID string
	if err := tx.GetContext(ctx, &assigneeID, `
		SELECT id::text
		FROM actors
		WHERE ap_id = $1
	`, assigneeAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}

	var projectParticipant bool
	if err := tx.GetContext(ctx, &projectParticipant, `
		SELECT EXISTS(
			SELECT 1
			FROM project_members
			WHERE user_id = $1 AND project_id = $2
		) OR EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, assigneeID, targetActorID); err != nil {
		return err
	}
	if !projectParticipant {
		return ErrForbiddenActor
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ticket_assignees (ticket_id, actor_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, storedTicket.ID, assigneeID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET updated_at = now()
		WHERE id = $1
	`, storedTicket.ID); err != nil {
		return err
	}

	return updateTicketAssignedToDocumentTx(ctx, tx, storedTicket.ID, ticketAPID)
}

func (r *PgRepository) deleteRemoteTicketAssigneeTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	assigneeAPID := *activity.ObjectAPID
	ticketAPID := *activity.TargetAPID

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	var storedTicket struct {
		ID         string `db:"id"`
		ProjectID  string `db:"project_id"`
		ReporterID string `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &storedTicket, `
		SELECT id::text, project_id::text, reporter_id::text
		FROM tickets
		WHERE ap_id = $1
		FOR UPDATE
	`, ticketAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}
	if storedTicket.ProjectID != targetActorID || storedTicket.ReporterID != activity.ActorID {
		return ErrActivityConflict
	}

	var assigneeID string
	if err := tx.GetContext(ctx, &assigneeID, `
		SELECT id::text
		FROM actors
		WHERE ap_id = $1
	`, assigneeAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM ticket_assignees
		WHERE ticket_id = $1 AND actor_id = $2
	`, storedTicket.ID, assigneeID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET updated_at = now()
		WHERE id = $1
	`, storedTicket.ID); err != nil {
		return err
	}

	return updateTicketAssignedToDocumentTx(ctx, tx, storedTicket.ID, ticketAPID)
}

func (r *PgRepository) deleteRemoteTicketTx(ctx context.Context, tx *sqlx.Tx, targetActorID string, activity *InboundActivity) error {
	ticketAPID := *activity.ObjectAPID

	var acceptedFollower bool
	if err := tx.GetContext(ctx, &acceptedFollower, `
		SELECT EXISTS(
			SELECT 1
			FROM actor_follows
			WHERE follower_actor_id = $1
				AND followed_actor_id = $2
				AND state = 'accepted'
		)
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}
	if !acceptedFollower {
		return ErrForbiddenActor
	}

	var storedTicket struct {
		ID         string `db:"id"`
		ProjectID  string `db:"project_id"`
		ReporterID string `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &storedTicket, `
		SELECT id::text, project_id::text, reporter_id::text
		FROM tickets
		WHERE ap_id = $1
		FOR UPDATE
	`, ticketAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}
	if storedTicket.ProjectID != targetActorID || storedTicket.ReporterID != activity.ActorID {
		return ErrActivityConflict
	}

	if err := tombstoneTicketCommentsTx(ctx, tx, storedTicket.ID); err != nil {
		return err
	}
	if err := tombstoneObjectTx(ctx, tx, ticketAPID, "forge:Ticket"); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM tickets
		WHERE id = $1
	`, storedTicket.ID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrInvalidActivity
	}
	return nil
}

func tombstoneTicketCommentsTx(ctx context.Context, tx *sqlx.Tx, ticketID string) error {
	var commentAPIDs []string
	if err := tx.SelectContext(ctx, &commentAPIDs, `
		SELECT ap_id
		FROM comments
		WHERE ticket_id = $1
	`, ticketID); err != nil {
		return err
	}
	for _, apID := range commentAPIDs {
		if err := tombstoneObjectTx(ctx, tx, apID, "Note"); err != nil {
			return err
		}
	}
	return nil
}

func tombstoneObjectTx(ctx context.Context, tx *sqlx.Tx, apID string, formerType string) error {
	rawDoc, err := json.Marshal(activitypub.TombstoneDocument(apID, formerType, time.Now().UTC()))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE ap_objects
		SET object_type = 'Tombstone',
			local_ref_table = NULL,
			local_ref_id = NULL,
			document = $2,
			is_deleted = true
		WHERE ap_id = $1
	`, apID, rawDoc)
	return err
}

func updateTicketAssignedToDocumentTx(ctx context.Context, tx *sqlx.Tx, ticketID, ticketAPID string) error {
	var rawDocument []byte
	if err := tx.GetContext(ctx, &rawDocument, `
		SELECT document
		FROM ap_objects
		WHERE ap_id = $1
		FOR UPDATE
	`, ticketAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidActivity
		}
		return err
	}

	var document map[string]any
	if err := json.Unmarshal(rawDocument, &document); err != nil {
		return err
	}

	var assigneeAPIDs []string
	if err := tx.SelectContext(ctx, &assigneeAPIDs, `
		SELECT actor.ap_id
		FROM ticket_assignees assignee
		JOIN actors actor ON actor.id = assignee.actor_id
		WHERE assignee.ticket_id = $1
		ORDER BY assignee.created_at ASC, actor.ap_id ASC
	`, ticketID); err != nil {
		return err
	}
	if len(assigneeAPIDs) == 0 {
		delete(document, "forge:assignedTo")
	} else {
		document["forge:assignedTo"] = assigneeAPIDs
	}

	updatedDocument, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE ap_objects
		SET document = $2,
			is_deleted = false
		WHERE ap_id = $1
	`, ticketAPID, updatedDocument)
	return err
}

func (r *PgRepository) AcceptProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) (*FollowResponse, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var targetActorAPID string
	var followerAPID string
	var followerInbox string
	if err := tx.QueryRowxContext(ctx, `
		SELECT target.ap_id, follower.ap_id, follower.inbox_url
		FROM actors target
		JOIN projects project ON project.id = target.id
		JOIN actors follower ON follower.id = $2
		WHERE target.id = $1
	`, targetActorID, activity.ActorID).Scan(&targetActorAPID, &followerAPID, &followerInbox); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnsupportedActivity
		}
		return nil, err
	}
	if followerAPID != activity.ActorAPID {
		return nil, ErrForbiddenActor
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
		ON CONFLICT (follower_actor_id, followed_actor_id)
		DO UPDATE SET state = 'accepted'
	`, activity.ActorID, targetActorID); err != nil {
		return nil, err
	}

	responseID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	responseAPID := activitypub.ActivityAPID(r.cfg, responseID)
	doc := activitypub.ActivityDocument("Accept", responseAPID, targetActorAPID, activity.ID, activity.ActorAPID, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Accept', $3, $4, $5, $6)
	`, responseID, responseAPID, targetActorID, activity.ID, activity.ActorAPID, rawDoc); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, targetActorID, responseID, responseAPID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &FollowResponse{
		ActivityID:     responseID,
		ActivityAPID:   responseAPID,
		TargetInboxURL: followerInbox,
	}, nil
}

func (r *PgRepository) UndoProjectFollow(ctx context.Context, targetActorID string, activity *InboundActivity) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var followerAPID string
	if err := tx.QueryRowxContext(ctx, `
		SELECT follower.ap_id
		FROM actors target
		JOIN projects project ON project.id = target.id
		JOIN actors follower ON follower.id = $2
		WHERE target.id = $1
	`, targetActorID, activity.ActorID).Scan(&followerAPID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnsupportedActivity
		}
		return err
	}
	if followerAPID != activity.ActorAPID {
		return ErrForbiddenActor
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, activity.ActorID, targetActorID); err != nil {
		return err
	}

	return tx.Commit()
}

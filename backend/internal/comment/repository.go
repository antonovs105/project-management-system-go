package comment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for comments and their ActivityPub records.
type Repository interface {
	Create(ctx context.Context, comment *Comment) (string, error)
	GetByID(ctx context.Context, commentID string) (*Comment, error)
	ListByTicketID(ctx context.Context, ticketID string) ([]Comment, error)
	Delete(ctx context.Context, commentID string, actorID string) (*DeleteResult, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

// NewRepository creates a PostgreSQL-backed comment repository.
func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{db: db, cfg: cfg}
}

// Create stores a comment, Note object, and Create activity in one transaction.
func (r *PgRepository) Create(ctx context.Context, comment *Comment) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO comments (id, ap_id, ticket_id, author_id, content)
		VALUES (:id, :ap_id, :ticket_id, :author_id, :content)
	`, comment); err != nil {
		return "", err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT created_at, updated_at FROM comments WHERE id = $1
	`, comment.ID).Scan(&comment.CreatedAt, &comment.UpdatedAt); err != nil {
		return "", err
	}

	var ticket struct {
		APID      string `db:"ap_id"`
		ProjectID string `db:"project_id"`
	}
	if err := tx.GetContext(ctx, &ticket, `
		SELECT ap_id, project_id::text
		FROM tickets
		WHERE id = $1
	`, comment.TicketID); err != nil {
		return "", err
	}
	authorAPID, err := lookupActorAPID(ctx, tx, comment.AuthorID)
	if err != nil {
		return "", err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, ticket.ProjectID)
	if err != nil {
		return "", err
	}

	noteDoc := activitypub.NoteDocument(comment.APID, ticket.APID, authorAPID, comment.Content, comment.CreatedAt)
	noteRaw, err := json.Marshal(noteDoc)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Note', $2, 'comments', $3, $4)
	`, comment.APID, comment.AuthorID, comment.ID, noteRaw); err != nil {
		return "", err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return "", err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	activityDoc := activitypub.ActivityDocument("Create", activityAPID, authorAPID, noteDoc, projectAPID, time.Now().UTC())
	activityRaw, err := json.Marshal(activityDoc)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Create', $3, $4, $5, $6)
	`, activityID, activityAPID, comment.AuthorID, comment.APID, projectAPID, activityRaw); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, comment.AuthorID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, ticket.ProjectID, activityID, activityAPID); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return activityID, nil
}

// ListByTicketID returns comments ordered by creation time for a ticket.
func (r *PgRepository) ListByTicketID(ctx context.Context, ticketID string) ([]Comment, error) {
	var comments []Comment
	if err := r.db.SelectContext(ctx, &comments, `
		SELECT id::text, ap_id, ticket_id::text, author_id::text, content, created_at, updated_at
		FROM comments
		WHERE ticket_id = $1
		ORDER BY created_at ASC
	`, ticketID); err != nil {
		return nil, err
	}
	return comments, nil
}

// GetByID returns a single comment by UUID.
func (r *PgRepository) GetByID(ctx context.Context, commentID string) (*Comment, error) {
	var comment Comment
	err := r.db.GetContext(ctx, &comment, `
		SELECT id::text, ap_id, ticket_id::text, author_id::text, content, created_at, updated_at
		FROM comments
		WHERE id = $1
	`, commentID)
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// Delete removes a comment and tombstones its ActivityPub object.
func (r *PgRepository) Delete(ctx context.Context, commentID string, actorID string) (*DeleteResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var stored struct {
		ID        string `db:"id"`
		APID      string `db:"ap_id"`
		TicketID  string `db:"ticket_id"`
		ProjectID string `db:"project_id"`
	}
	if err := tx.GetContext(ctx, &stored, `
		SELECT
			comment.id::text,
			comment.ap_id,
			comment.ticket_id::text,
			ticket.project_id::text
		FROM comments comment
		JOIN tickets ticket ON ticket.id = comment.ticket_id
		WHERE comment.id = $1
		FOR UPDATE OF comment
	`, commentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("comment not found")
		}
		return nil, err
	}

	recipientInboxes, err := remoteTicketRecipientInboxes(ctx, tx, stored.ProjectID, stored.TicketID)
	if err != nil {
		return nil, err
	}
	if err := tombstoneObject(ctx, tx, stored.APID, "Note"); err != nil {
		return nil, err
	}

	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return nil, err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, stored.ProjectID)
	if err != nil {
		return nil, err
	}
	activityID, err := activitypub.NewID()
	if err != nil {
		return nil, err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	activityDoc := activitypub.ActivityDocument("Delete", activityAPID, actorAPID, stored.APID, projectAPID, time.Now().UTC())
	activityRaw, err := json.Marshal(activityDoc)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Delete', $3, $4, $5, $6)
	`, activityID, activityAPID, actorID, stored.APID, projectAPID, activityRaw); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, stored.ProjectID, activityID, activityAPID); err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = $1`, stored.ID)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errors.New("comment not found")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{
		ActivityID:       activityID,
		ProjectID:        stored.ProjectID,
		RecipientInboxes: recipientInboxes,
	}, nil
}

func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

func tombstoneObject(ctx context.Context, q sqlx.ExecerContext, apID string, formerType string) error {
	rawDoc, err := json.Marshal(activitypub.TombstoneDocument(apID, formerType, time.Now().UTC()))
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
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

func remoteTicketRecipientInboxes(ctx context.Context, q sqlx.QueryerContext, projectID string, ticketID string) ([]string, error) {
	var inboxes []string
	err := sqlx.SelectContext(ctx, q, &inboxes, `
		WITH recipients AS (
			SELECT follower.inbox_url
			FROM actor_follows f
			JOIN actors follower ON follower.id = f.follower_actor_id
			WHERE f.followed_actor_id = $1
				AND f.state = 'accepted'
				AND follower.is_local = false
				AND follower.inbox_url <> ''

			UNION

			SELECT reporter.inbox_url
			FROM tickets ticket
			JOIN actors reporter ON reporter.id = ticket.reporter_id
			WHERE ticket.project_id = $1
				AND ticket.id = $2
				AND reporter.is_local = false
				AND reporter.inbox_url <> ''

			UNION

			SELECT assignee.inbox_url
			FROM tickets ticket
			JOIN ticket_assignees ta ON ta.ticket_id = ticket.id
			JOIN actors assignee ON assignee.id = ta.actor_id
			WHERE ticket.project_id = $1
				AND ticket.id = $2
				AND assignee.is_local = false
				AND assignee.inbox_url <> ''

			UNION

			SELECT author.inbox_url
			FROM comments comment
			JOIN actors author ON author.id = comment.author_id
			WHERE comment.ticket_id = $2
				AND author.is_local = false
				AND author.inbox_url <> ''
		)
		SELECT DISTINCT inbox_url
		FROM recipients
		ORDER BY inbox_url ASC
	`, projectID, ticketID)
	return inboxes, err
}

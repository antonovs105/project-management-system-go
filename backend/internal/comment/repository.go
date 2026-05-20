package comment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, comment *Comment) error
	ListByTicketID(ctx context.Context, ticketID string) ([]Comment, error)
}

type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{db: db, cfg: cfg}
}

func (r *PgRepository) Create(ctx context.Context, comment *Comment) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO comments (id, ap_id, ticket_id, author_id, content)
		VALUES (:id, :ap_id, :ticket_id, :author_id, :content)
	`, comment); err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT created_at, updated_at FROM comments WHERE id = $1
	`, comment.ID).Scan(&comment.CreatedAt, &comment.UpdatedAt); err != nil {
		return err
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
		return err
	}
	authorAPID, err := lookupActorAPID(ctx, tx, comment.AuthorID)
	if err != nil {
		return err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, ticket.ProjectID)
	if err != nil {
		return err
	}

	noteDoc := activitypub.NoteDocument(comment.APID, ticket.APID, authorAPID, comment.Content, comment.CreatedAt)
	noteRaw, err := json.Marshal(noteDoc)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Note', $2, 'comments', $3, $4)
	`, comment.APID, comment.AuthorID, comment.ID, noteRaw); err != nil {
		return err
	}

	activityID, err := activitypub.NewID()
	if err != nil {
		return err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	activityDoc := activitypub.ActivityDocument("Create", activityAPID, authorAPID, noteDoc, projectAPID, time.Now().UTC())
	activityRaw, err := json.Marshal(activityDoc)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, 'Create', $3, $4, $5, $6)
	`, activityID, activityAPID, comment.AuthorID, comment.APID, projectAPID, activityRaw); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, comment.AuthorID, activityID, activityAPID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, ticket.ProjectID, activityID, activityAPID); err != nil {
		return err
	}

	return tx.Commit()
}

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

func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

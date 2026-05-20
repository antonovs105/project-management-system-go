package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
)

type Repository interface {
	Create(ctx context.Context, ticket *Ticket) ([]string, error)
	ListByProjectID(ctx context.Context, projectID string) ([]Ticket, error)
	GetByID(ctx context.Context, id string) (*Ticket, error)
	Update(ctx context.Context, ticket *Ticket) ([]string, error)
	Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error)
	CreateLink(ctx context.Context, link *TicketLink) error
	DeleteLink(ctx context.Context, linkID string) error
	GetLinksByProjectID(ctx context.Context, projectID string) ([]TicketLink, error)
}

type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{db: db, cfg: cfg}
}

func (r *PgRepository) Create(ctx context.Context, ticket *Ticket) ([]string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ticket.IsResolved = ticket.Status == "done"
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO tickets (
			id, ap_id, title, description, status, priority, type, parent_id,
			project_id, reporter_id, is_resolved, resolved_at, resolved_by_actor_id
		)
		VALUES (
			:id, :ap_id, :title, :description, :status, :priority, :type, :parent_id,
			:project_id, :reporter_id, :is_resolved,
			CASE WHEN :is_resolved THEN now() ELSE NULL END,
			CASE WHEN :is_resolved THEN CAST(:reporter_id AS uuid) ELSE NULL END
		)
	`, ticket); err != nil {
		return nil, err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT created_at, updated_at FROM tickets WHERE id = $1
	`, ticket.ID).Scan(&ticket.CreatedAt, &ticket.UpdatedAt); err != nil {
		return nil, err
	}

	if ticket.AssigneeID != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ticket_assignees (ticket_id, actor_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, ticket.ID, *ticket.AssigneeID); err != nil {
			return nil, err
		}
	}

	if err := r.writeTicketObject(ctx, tx, ticket); err != nil {
		return nil, err
	}
	activityID, err := r.writeTicketActivity(ctx, tx, "Create", ticket, ticket.ReporterID, ticket.APID, nil)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []string{activityID}, nil
}

func (r *PgRepository) ListByProjectID(ctx context.Context, projectID string) ([]Ticket, error) {
	var tickets []Ticket
	query := ticketSelectBase() + `
		WHERE t.project_id = $1
		ORDER BY t.created_at DESC
	`
	if err := r.db.SelectContext(ctx, &tickets, query, projectID); err != nil {
		return nil, err
	}
	return tickets, nil
}

func (r *PgRepository) GetByID(ctx context.Context, id string) (*Ticket, error) {
	var t Ticket
	query := ticketSelectBase() + ` WHERE t.id = $1`
	err := r.db.GetContext(ctx, &t, query, id)
	return &t, err
}

func (r *PgRepository) Update(ctx context.Context, ticket *Ticket) ([]string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ticket.IsResolved = ticket.Status == "done"
	result, err := tx.NamedExecContext(ctx, `
		UPDATE tickets
		SET
			title = :title,
			description = :description,
			status = :status,
			priority = :priority,
			type = :type,
			parent_id = :parent_id,
			is_resolved = :is_resolved,
			resolved_at = CASE
				WHEN :is_resolved AND resolved_at IS NULL THEN now()
				WHEN NOT :is_resolved THEN NULL
				ELSE resolved_at
			END,
			resolved_by_actor_id = CASE
				WHEN :is_resolved THEN CAST(:reporter_id AS uuid)
				ELSE NULL
			END
		WHERE id = :id
	`, ticket)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errors.New("ticket to update not found")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM ticket_assignees WHERE ticket_id = $1`, ticket.ID); err != nil {
		return nil, err
	}
	if ticket.AssigneeID != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ticket_assignees (ticket_id, actor_id)
			VALUES ($1, $2)
		`, ticket.ID, *ticket.AssigneeID); err != nil {
			return nil, err
		}
	}

	if err := r.writeTicketObject(ctx, tx, ticket); err != nil {
		return nil, err
	}
	activityIDs := make([]string, 0, 2)
	updateActivityID, err := r.writeTicketActivity(ctx, tx, "Update", ticket, ticket.ReporterID, ticket.APID, nil)
	if err != nil {
		return nil, err
	}
	activityIDs = append(activityIDs, updateActivityID)
	if ticket.AssigneeID != nil {
		assigneeAPID, err := lookupActorAPID(ctx, tx, *ticket.AssigneeID)
		if err != nil {
			return nil, err
		}
		addActivityID, err := r.writeTicketActivity(ctx, tx, "Add", ticket, ticket.ReporterID, assigneeAPID, ticket.APID)
		if err != nil {
			return nil, err
		}
		activityIDs = append(activityIDs, addActivityID)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return activityIDs, nil
}

func (r *PgRepository) Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error) {
	t, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	recipientInboxes, err := r.remoteTicketRecipientInboxes(ctx, tx, t.ProjectID, t.ID)
	if err != nil {
		return nil, err
	}
	if err := tombstoneTicketComments(ctx, tx, t.ID); err != nil {
		return nil, err
	}
	if err := tombstoneObject(ctx, tx, t.APID, "forge:Ticket"); err != nil {
		return nil, err
	}
	activityID, err := r.writeTicketActivity(ctx, tx, "Delete", t, actorID, t.APID, nil)
	if err != nil {
		return nil, err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errors.New("ticket to delete not found")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &DeleteResult{
		ActivityIDs:      []string{activityID},
		RecipientInboxes: recipientInboxes,
	}, nil
}

func (r *PgRepository) CreateLink(ctx context.Context, link *TicketLink) error {
	query := `
		INSERT INTO ticket_links (source_id, target_id, link_type)
		VALUES (:source_id, :target_id, :link_type)
		RETURNING id::text, created_at
	`
	rows, err := r.db.NamedQueryContext(ctx, query, link)
	if err != nil {
		return err
	}
	defer rows.Close()

	if rows.Next() {
		return rows.Scan(&link.ID, &link.CreatedAt)
	}
	return errors.New("link creation failed")
}

func (r *PgRepository) DeleteLink(ctx context.Context, linkID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM ticket_links WHERE id = $1`, linkID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("link not found")
	}
	return nil
}

func (r *PgRepository) GetLinksByProjectID(ctx context.Context, projectID string) ([]TicketLink, error) {
	var links []TicketLink
	query := `
		SELECT
			l.id::text,
			l.source_id::text,
			l.target_id::text,
			l.link_type,
			l.created_at
		FROM ticket_links l
		JOIN tickets t ON l.source_id = t.id
		WHERE t.project_id = $1
	`
	if err := r.db.SelectContext(ctx, &links, query, projectID); err != nil {
		return nil, err
	}
	return links, nil
}

func (r *PgRepository) writeTicketObject(ctx context.Context, tx *sqlx.Tx, ticket *Ticket) error {
	projectAPID, err := lookupActorAPID(ctx, tx, ticket.ProjectID)
	if err != nil {
		return err
	}
	reporterAPID, err := lookupActorAPID(ctx, tx, ticket.ReporterID)
	if err != nil {
		return err
	}
	assignees, err := lookupAssigneeAPIDs(ctx, tx, ticket.ID)
	if err != nil {
		return err
	}
	var parentAPID *string
	if ticket.ParentID != nil {
		var value string
		if err := tx.GetContext(ctx, &value, `SELECT ap_id FROM tickets WHERE id = $1`, *ticket.ParentID); err != nil {
			return err
		}
		parentAPID = &value
	}

	doc := activitypub.TicketDocument(
		ticket.APID,
		projectAPID,
		reporterAPID,
		ticket.Title,
		ticket.Description,
		ticket.Status,
		ticket.Priority,
		ticket.Type,
		parentAPID,
		assignees,
		ticket.CreatedAt,
		ticket.IsResolved,
	)
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Ticket', $2, 'tickets', $3, $4)
		ON CONFLICT (ap_id) DO UPDATE
		SET document = EXCLUDED.document,
			object_type = EXCLUDED.object_type,
			actor_id = EXCLUDED.actor_id,
			local_ref_table = EXCLUDED.local_ref_table,
			local_ref_id = EXCLUDED.local_ref_id,
			is_deleted = false
	`, ticket.APID, ticket.ReporterID, ticket.ID, rawDoc)
	return err
}

func (r *PgRepository) writeTicketActivity(ctx context.Context, tx *sqlx.Tx, activityType string, ticket *Ticket, actorID string, object any, target any) (string, error) {
	activityID, err := activitypub.NewID()
	if err != nil {
		return "", err
	}
	activityAPID := activitypub.ActivityAPID(r.cfg, activityID)
	actorAPID, err := lookupActorAPID(ctx, tx, actorID)
	if err != nil {
		return "", err
	}
	projectAPID, err := lookupActorAPID(ctx, tx, ticket.ProjectID)
	if err != nil {
		return "", err
	}
	if target == nil {
		target = projectAPID
	}
	doc := activitypub.ActivityDocument(activityType, activityAPID, actorAPID, object, target, time.Now().UTC())
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	targetAPID := ""
	if value, ok := target.(string); ok {
		targetAPID = value
	}
	objectAPID := ""
	if value, ok := object.(string); ok {
		objectAPID = value
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, target_ap_id, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, activityID, activityAPID, activityType, actorID, objectAPID, targetAPID, rawDoc); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, activityID, activityAPID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO actor_inbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, ticket.ProjectID, activityID, activityAPID); err != nil {
		return "", err
	}
	return activityID, nil
}

func ticketSelectBase() string {
	return `
		SELECT
			t.id::text,
			t.ap_id,
			t.title,
			t.description,
			t.status,
			t.priority,
			t.type,
			t.parent_id::text,
			t.project_id::text,
			t.reporter_id::text,
			ta.actor_id::text AS assignee_id,
			t.is_resolved,
			t.created_at,
			t.updated_at
		FROM tickets t
		LEFT JOIN LATERAL (
			SELECT actor_id
			FROM ticket_assignees
			WHERE ticket_id = t.id
			ORDER BY created_at ASC
			LIMIT 1
		) ta ON true
	`
}

func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

func lookupAssigneeAPIDs(ctx context.Context, q sqlx.QueryerContext, ticketID string) ([]string, error) {
	var apIDs []string
	err := sqlx.SelectContext(ctx, q, &apIDs, `
		SELECT a.ap_id
		FROM ticket_assignees ta
		JOIN actors a ON a.id = ta.actor_id
		WHERE ta.ticket_id = $1
		ORDER BY ta.created_at ASC
	`, ticketID)
	return apIDs, err
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

func tombstoneTicketComments(ctx context.Context, tx *sqlx.Tx, ticketID string) error {
	var commentAPIDs []string
	if err := tx.SelectContext(ctx, &commentAPIDs, `
		SELECT ap_id
		FROM comments
		WHERE ticket_id = $1
	`, ticketID); err != nil {
		return err
	}
	for _, apID := range commentAPIDs {
		if err := tombstoneObject(ctx, tx, apID, "Note"); err != nil {
			return err
		}
	}
	return nil
}

func (r *PgRepository) remoteTicketRecipientInboxes(ctx context.Context, q sqlx.QueryerContext, projectID string, ticketID string) ([]string, error) {
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

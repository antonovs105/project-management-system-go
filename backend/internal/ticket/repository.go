package ticket

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/lexorank"
	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for tickets and ticket links.
type Repository interface {
	Create(ctx context.Context, ticket *Ticket) (*ActivityResult, error)
	ListByProjectID(ctx context.Context, projectID string, options TicketListOptions) ([]Ticket, error)
	GetByID(ctx context.Context, id string) (*Ticket, error)
	Update(ctx context.Context, ticket *Ticket, actorID string) (*ActivityResult, error)
	Move(ctx context.Context, ticketID, actorID, status string, beforeTicketID, afterTicketID *string) (*Ticket, *ActivityResult, error)
	Delete(ctx context.Context, id string, actorID string) (*DeleteResult, error)
	CreateLink(ctx context.Context, link *TicketLink) error
	GetLinkByID(ctx context.Context, linkID string) (*TicketLink, error)
	DeleteLink(ctx context.Context, linkID string) error
	GetLinksByProjectID(ctx context.Context, projectID string) ([]TicketLink, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

// NewRepository creates a PostgreSQL-backed ticket repository.
func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{db: db, cfg: cfg}
}

// Create stores a ticket, its ForgeFed object, and a Create activity.
func (r *PgRepository) Create(ctx context.Context, ticket *Ticket) (*ActivityResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ticket.IsResolved = ticket.Status == "done"
	rank, err := r.rankAtEnd(ctx, tx, ticket.ProjectID, ticket.Status, "")
	if err != nil {
		return nil, err
	}
	ticket.Rank = rank
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO tickets (
			id, ap_id, title, description, status, priority, type, rank, parent_id,
			project_id, reporter_id, is_resolved, resolved_at, resolved_by_actor_id
		)
		VALUES (
			:id, :ap_id, :title, :description, :status, :priority, :type, :rank, :parent_id,
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
		if err := ensureProjectParticipant(ctx, tx, ticket.ProjectID, *ticket.AssigneeID); err != nil {
			return nil, err
		}
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
	inboxes, err := r.remoteTicketRecipientInboxes(ctx, tx, ticket.ProjectID, ticket.ID)
	if err != nil {
		return nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", inboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ActivityResult{ActivityIDs: []string{activityID}, Deliveries: deliveries}, nil
}

// ListByProjectID returns tickets for a project.
func (r *PgRepository) ListByProjectID(ctx context.Context, projectID string, options TicketListOptions) ([]Ticket, error) {
	tickets := make([]Ticket, 0)
	conditions := []string{"t.project_id = $1"}
	args := []any{projectID}
	if options.AssigneeID != nil {
		args = append(args, *options.AssigneeID)
		conditions = append(conditions, fmt.Sprintf("ta.actor_id = $%d::uuid", len(args)))
	}
	if options.Unassigned {
		conditions = append(conditions, "ta.actor_id IS NULL")
	}
	args = append(args, options.Limit, options.Offset)
	limitPos := len(args) - 1
	offsetPos := len(args)

	query := ticketSelectBase() + `
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY
			CASE t.status
				WHEN 'open' THEN 1
				WHEN 'in_progress' THEN 2
				WHEN 'review' THEN 3
				WHEN 'done' THEN 4
				ELSE 5
			END ASC,
			t.rank ASC,
			t.created_at ASC,
			t.id ASC
		LIMIT $` + fmt.Sprint(limitPos) + ` OFFSET $` + fmt.Sprint(offsetPos) + `
	`
	if err := r.db.SelectContext(ctx, &tickets, query, args...); err != nil {
		return nil, err
	}
	return tickets, nil
}

// GetByID loads a ticket by UUID.
func (r *PgRepository) GetByID(ctx context.Context, id string) (*Ticket, error) {
	var t Ticket
	query := ticketSelectBase() + ` WHERE t.id = $1`
	err := r.db.GetContext(ctx, &t, query, id)
	return &t, err
}

// Update changes a ticket and records ActivityPub activities for the change.
func (r *PgRepository) Update(ctx context.Context, ticket *Ticket, actorID string) (*ActivityResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var current struct {
		Status     string `db:"status"`
		IsResolved bool   `db:"is_resolved"`
	}
	if err := tx.GetContext(ctx, &current, `
		SELECT status, is_resolved
		FROM tickets
		WHERE id = $1
		FOR UPDATE
	`, ticket.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("ticket to update not found")
		}
		return nil, err
	}

	existingAssigneeIDs, err := lookupAssigneeActorIDs(ctx, tx, ticket.ID)
	if err != nil {
		return nil, err
	}
	if ticket.AssigneeID != nil {
		if err := ensureProjectParticipant(ctx, tx, ticket.ProjectID, *ticket.AssigneeID); err != nil {
			return nil, err
		}
	}
	if current.Status != ticket.Status {
		rank, err := r.rankAtEnd(ctx, tx, ticket.ProjectID, ticket.Status, ticket.ID)
		if err != nil {
			return nil, err
		}
		ticket.Rank = rank
	}

	ticket.IsResolved = ticket.Status == "done"
	result, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET
			title = $2,
			description = $3,
			status = $4,
			priority = $5,
			type = $6,
			rank = $7,
			parent_id = $8,
			is_resolved = $9,
			resolved_at = CASE
				WHEN $9 AND resolved_at IS NULL THEN now()
				WHEN NOT $9 THEN NULL
				ELSE resolved_at
			END,
			resolved_by_actor_id = CASE
				WHEN $9 AND NOT $10 THEN CAST($11 AS uuid)
				WHEN $9 THEN resolved_by_actor_id
				ELSE NULL
			END
		WHERE id = $1
	`,
		ticket.ID,
		ticket.Title,
		ticket.Description,
		ticket.Status,
		ticket.Priority,
		ticket.Type,
		ticket.Rank,
		ticket.ParentID,
		ticket.IsResolved,
		current.IsResolved,
		actorID,
	)
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
	updateActivityID, err := r.writeTicketActivity(ctx, tx, "Update", ticket, actorID, ticket.APID, nil)
	if err != nil {
		return nil, err
	}
	activityIDs = append(activityIDs, updateActivityID)
	if ticket.AssigneeID != nil && !containsString(existingAssigneeIDs, *ticket.AssigneeID) {
		assigneeAPID, err := lookupActorAPID(ctx, tx, *ticket.AssigneeID)
		if err != nil {
			return nil, err
		}
		addActivityID, err := r.writeTicketActivity(ctx, tx, "Add", ticket, actorID, assigneeAPID, ticket.APID)
		if err != nil {
			return nil, err
		}
		activityIDs = append(activityIDs, addActivityID)
	}
	inboxes, err := r.remoteTicketRecipientInboxes(ctx, tx, ticket.ProjectID, ticket.ID)
	if err != nil {
		return nil, err
	}
	deliveries := make([]apdelivery.QueueCandidate, 0, len(activityIDs)*len(inboxes))
	for _, activityID := range activityIDs {
		activityDeliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", inboxes, apdelivery.DefaultMaxRetry)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, activityDeliveries...)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ActivityResult{ActivityIDs: activityIDs, Deliveries: deliveries}, nil
}

// Move changes a ticket status and rank using neighbor ticket boundaries.
func (r *PgRepository) Move(ctx context.Context, ticketID, actorID, status string, beforeTicketID, afterTicketID *string) (*Ticket, *ActivityResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var current struct {
		ProjectID  string `db:"project_id"`
		IsResolved bool   `db:"is_resolved"`
		ReporterID string `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &current, `
		SELECT project_id::text, is_resolved, reporter_id::text
		FROM tickets
		WHERE id = $1
		FOR UPDATE
	`, ticketID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errors.New("ticket to move not found")
		}
		return nil, nil, err
	}

	rank, err := r.rankBetween(ctx, tx, current.ProjectID, status, ticketID, beforeTicketID, afterTicketID)
	if err != nil {
		return nil, nil, err
	}
	isResolved := status == "done"
	result, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET
			status = $2,
			rank = $3,
			is_resolved = $4,
			resolved_at = CASE
				WHEN $4 AND resolved_at IS NULL THEN now()
				WHEN NOT $4 THEN NULL
				ELSE resolved_at
			END,
			resolved_by_actor_id = CASE
				WHEN $4 AND NOT $5 THEN CAST($6 AS uuid)
				WHEN $4 THEN resolved_by_actor_id
				ELSE NULL
			END
		WHERE id = $1
	`, ticketID, status, rank, isResolved, current.IsResolved, actorID)
	if err != nil {
		return nil, nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	if rowsAffected == 0 {
		return nil, nil, errors.New("ticket to move not found")
	}

	moved, err := r.getByIDInTx(ctx, tx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	if err := r.writeTicketObject(ctx, tx, moved); err != nil {
		return nil, nil, err
	}
	activityID, err := r.writeTicketActivity(ctx, tx, "Update", moved, actorID, moved.APID, nil)
	if err != nil {
		return nil, nil, err
	}
	inboxes, err := r.remoteTicketRecipientInboxes(ctx, tx, moved.ProjectID, moved.ID)
	if err != nil {
		return nil, nil, err
	}
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", inboxes, apdelivery.DefaultMaxRetry)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return moved, &ActivityResult{ActivityIDs: []string{activityID}, Deliveries: deliveries}, nil
}

// Delete removes a ticket and tombstones its related ActivityPub objects.
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
	deliveries, err := apdelivery.CreateRowsForInboxes(ctx, tx, activityID, "", recipientInboxes, apdelivery.DefaultMaxRetry)
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
		Deliveries:       deliveries,
	}, nil
}

// CreateLink stores a directed link between tickets.
func (r *PgRepository) CreateLink(ctx context.Context, link *TicketLink) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sourceProjectID string
	if err := tx.GetContext(ctx, &sourceProjectID, `
		SELECT project_id::text
		FROM tickets
		WHERE id = $1
	`, link.SourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return invalidTicketInput("source ticket not found")
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, sourceProjectID); err != nil {
		return err
	}

	var targetProjectID string
	if err := tx.GetContext(ctx, &targetProjectID, `
		SELECT project_id::text
		FROM tickets
		WHERE id = $1
	`, link.TargetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return invalidTicketInput("target ticket not found")
		}
		return err
	}
	if targetProjectID != sourceProjectID {
		return invalidTicketInput("cannot link tickets from different projects")
	}

	var cycle bool
	if err := tx.GetContext(ctx, &cycle, `
		WITH RECURSIVE reachable(id) AS (
			SELECT CAST($2 AS uuid)
			UNION
			SELECT ticket_link.target_id
			FROM ticket_links ticket_link
			JOIN reachable ON reachable.id = ticket_link.source_id
		)
		SELECT EXISTS(
			SELECT 1
			FROM reachable
			WHERE id = CAST($1 AS uuid)
		)
	`, link.SourceID, link.TargetID); err != nil {
		return err
	}
	if cycle {
		return invalidTicketInput("cycle detected: path already exists from target to source")
	}

	query := `
		INSERT INTO ticket_links (source_id, target_id, link_type)
		VALUES ($1, $2, $3)
		RETURNING id::text, created_at
	`
	if err := tx.QueryRowxContext(ctx, query, link.SourceID, link.TargetID, link.LinkType).Scan(&link.ID, &link.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// GetLinkByID loads a ticket link by UUID.
func (r *PgRepository) GetLinkByID(ctx context.Context, linkID string) (*TicketLink, error) {
	var link TicketLink
	if err := r.db.GetContext(ctx, &link, `
		SELECT
			id::text,
			source_id::text,
			target_id::text,
			link_type,
			created_at
		FROM ticket_links
		WHERE id = $1
	`, linkID); err != nil {
		return nil, err
	}
	return &link, nil
}

// DeleteLink removes a ticket link by UUID.
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

// GetLinksByProjectID returns all links between tickets in a project.
func (r *PgRepository) GetLinksByProjectID(ctx context.Context, projectID string) ([]TicketLink, error) {
	links := make([]TicketLink, 0)
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

// getByIDInTx loads a ticket projection inside an existing transaction.
func (r *PgRepository) getByIDInTx(ctx context.Context, tx *sqlx.Tx, id string) (*Ticket, error) {
	var t Ticket
	query := ticketSelectBase() + ` WHERE t.id = $1`
	if err := tx.GetContext(ctx, &t, query, id); err != nil {
		return nil, err
	}
	return &t, nil
}

// rankAtEnd returns a rank after the current last ticket in a status group.
func (r *PgRepository) rankAtEnd(ctx context.Context, tx *sqlx.Tx, projectID, status, excludedTicketID string) (string, error) {
	if err := lockTicketRankGroup(ctx, tx, projectID, status); err != nil {
		return "", err
	}
	rank, err := rankAtEndLocked(ctx, tx, projectID, status, excludedTicketID)
	if errors.Is(err, lexorank.ErrNoSpace) {
		if err := rebalanceStatusRanks(ctx, tx, projectID, status, excludedTicketID); err != nil {
			return "", err
		}
		return rankAtEndLocked(ctx, tx, projectID, status, excludedTicketID)
	}
	return rank, err
}

// rankBetween returns a rank between optional before and after boundary tickets.
func (r *PgRepository) rankBetween(ctx context.Context, tx *sqlx.Tx, projectID, status, ticketID string, beforeTicketID, afterTicketID *string) (string, error) {
	if err := lockTicketRankGroup(ctx, tx, projectID, status); err != nil {
		return "", err
	}
	rank, err := rankBetweenLocked(ctx, tx, projectID, status, ticketID, beforeTicketID, afterTicketID)
	if errors.Is(err, lexorank.ErrNoSpace) {
		if err := rebalanceStatusRanks(ctx, tx, projectID, status, ticketID); err != nil {
			return "", err
		}
		return rankBetweenLocked(ctx, tx, projectID, status, ticketID, beforeTicketID, afterTicketID)
	}
	return rank, err
}

// rankAtEndLocked computes an end rank after the caller has locked the rank group.
func rankAtEndLocked(ctx context.Context, tx *sqlx.Tx, projectID, status, excludedTicketID string) (string, error) {
	query := `
		SELECT rank
		FROM tickets
		WHERE project_id = $1
			AND status = $2
			AND ($3 = '' OR id <> $3::uuid)
		ORDER BY rank DESC, created_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`
	var lastRank string
	if err := tx.GetContext(ctx, &lastRank, query, projectID, status, excludedTicketID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return lexorank.Initial(), nil
		}
		return "", err
	}
	return lexorank.Between(lastRank, "")
}

// rankBetweenLocked computes a neighbor rank after the caller has locked the rank group.
func rankBetweenLocked(ctx context.Context, tx *sqlx.Tx, projectID, status, ticketID string, beforeTicketID, afterTicketID *string) (string, error) {
	if beforeTicketID != nil && afterTicketID != nil && strings.TrimSpace(*beforeTicketID) == strings.TrimSpace(*afterTicketID) {
		return "", invalidTicketInput("move boundaries must be different tickets")
	}
	if beforeTicketID == nil && afterTicketID == nil {
		return rankAtEndLocked(ctx, tx, projectID, status, ticketID)
	}

	var prevRank string
	if afterTicketID != nil {
		rank, err := boundaryRank(ctx, tx, projectID, status, ticketID, *afterTicketID, "after")
		if err != nil {
			return "", err
		}
		prevRank = rank
	}
	var nextRank string
	if beforeTicketID != nil {
		rank, err := boundaryRank(ctx, tx, projectID, status, ticketID, *beforeTicketID, "before")
		if err != nil {
			return "", err
		}
		nextRank = rank
	}
	rank, err := lexorank.Between(prevRank, nextRank)
	if err != nil && !errors.Is(err, lexorank.ErrNoSpace) {
		return "", invalidTicketInput("move boundaries are not in board order")
	}
	return rank, err
}

// boundaryRank validates a move boundary ticket and returns its rank.
func boundaryRank(ctx context.Context, tx *sqlx.Tx, projectID, status, movingTicketID, boundaryTicketID, label string) (string, error) {
	boundaryTicketID = strings.TrimSpace(boundaryTicketID)
	if boundaryTicketID == "" {
		return "", invalidTicketInput(label + " ticket id is required")
	}
	if boundaryTicketID == movingTicketID {
		return "", invalidTicketInput("ticket cannot be moved relative to itself")
	}

	var rank string
	if err := tx.GetContext(ctx, &rank, `
		SELECT rank
		FROM tickets
		WHERE id = $1
			AND project_id = $2
			AND status = $3
		FOR UPDATE
	`, boundaryTicketID, projectID, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", invalidTicketInput(label + " ticket must be in the target status")
		}
		return "", err
	}
	return rank, nil
}

// lockTicketRankGroup serializes rank mutations for one project status column.
func lockTicketRankGroup(ctx context.Context, tx *sqlx.Tx, projectID, status string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID+":"+status)
	return err
}

// rebalanceStatusRanks redistributes ranks when a status group has no midpoint.
func rebalanceStatusRanks(ctx context.Context, tx *sqlx.Tx, projectID, status, excludedTicketID string) error {
	ids := make([]string, 0)
	if err := tx.SelectContext(ctx, &ids, `
		SELECT id::text
		FROM tickets
		WHERE project_id = $1
			AND status = $2
			AND ($3 = '' OR id <> $3::uuid)
		ORDER BY rank ASC, created_at ASC, id ASC
		FOR UPDATE
	`, projectID, status, excludedTicketID); err != nil {
		return err
	}
	ranks := lexorank.EvenlySpaced(len(ids))
	for index, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE tickets SET rank = $2 WHERE id = $1`, id, ranks[index]); err != nil {
			return err
		}
	}
	return nil
}

// writeTicketObject writes the current ForgeFed Ticket JSON-LD snapshot.
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

// writeTicketActivity stores an ActivityStreams activity for a ticket change.
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

// ticketSelectBase returns the shared ticket projection query.
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
			t.rank,
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

// lookupActorAPID resolves an actor UUID to its ActivityPub ID.
func lookupActorAPID(ctx context.Context, q sqlx.QueryerContext, actorID string) (string, error) {
	var apID string
	err := sqlx.GetContext(ctx, q, &apID, `SELECT ap_id FROM actors WHERE id = $1`, actorID)
	return apID, err
}

// lookupAssigneeAPIDs returns ActivityPub IDs assigned to a ticket.
func lookupAssigneeAPIDs(ctx context.Context, q sqlx.QueryerContext, ticketID string) ([]string, error) {
	apIDs := make([]string, 0)
	err := sqlx.SelectContext(ctx, q, &apIDs, `
		SELECT a.ap_id
		FROM ticket_assignees ta
		JOIN actors a ON a.id = ta.actor_id
		WHERE ta.ticket_id = $1
		ORDER BY ta.created_at ASC
	`, ticketID)
	return apIDs, err
}

// lookupAssigneeActorIDs returns actor UUIDs assigned to a ticket.
func lookupAssigneeActorIDs(ctx context.Context, q sqlx.QueryerContext, ticketID string) ([]string, error) {
	actorIDs := make([]string, 0)
	err := sqlx.SelectContext(ctx, q, &actorIDs, `
		SELECT actor_id::text
		FROM ticket_assignees
		WHERE ticket_id = $1
		ORDER BY created_at ASC, actor_id ASC
	`, ticketID)
	return actorIDs, err
}

// ensureProjectParticipant validates that an actor can be assigned in a project.
func ensureProjectParticipant(ctx context.Context, q sqlx.QueryerContext, projectID string, actorID string) error {
	var participant bool
	if err := sqlx.GetContext(ctx, q, &participant, `
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
	`, actorID, projectID); err != nil {
		return err
	}
	if !participant {
		return errors.New("assignee must be a project participant")
	}
	return nil
}

// containsString reports whether a string slice contains target.
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// tombstoneObject replaces a stored ActivityPub object with a Tombstone document.
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

// tombstoneTicketComments tombstones all comment objects attached to a ticket.
func tombstoneTicketComments(ctx context.Context, tx *sqlx.Tx, ticketID string) error {
	commentAPIDs := make([]string, 0)
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

// remoteTicketRecipientInboxes returns remote inboxes related to a ticket.
func (r *PgRepository) remoteTicketRecipientInboxes(ctx context.Context, q sqlx.QueryerContext, projectID string, ticketID string) ([]string, error) {
	inboxes := make([]string, 0)
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

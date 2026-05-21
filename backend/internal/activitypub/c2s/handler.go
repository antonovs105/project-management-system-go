package c2s

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
)

const maxOutboxBodyBytes = int64(1 << 20)

var (
	errUnsupportedActivity = errors.New("unsupported activity")
	errInvalidActivity     = errors.New("invalid activity")
	errActorMismatch       = errors.New("activity actor does not match authenticated user")
)

// TicketCreator creates local tickets from ActivityPub client activities.
type TicketCreator interface {
	CreateTicket(ctx context.Context, req ticket.CreateTicketRequest, projectID, reporterID string) (*ticket.Ticket, error)
}

// CommentCreator creates local comments from ActivityPub client activities.
type CommentCreator interface {
	CreateComment(ctx context.Context, ticketID, authorID, content string) (*comment.Comment, error)
}

// Repository resolves local ActivityPub IDs needed by the C2S outbox handler.
type Repository interface {
	LocalUser(ctx context.Context, userID string) (*localUser, error)
	LocalTicketID(ctx context.Context, apID string) (string, error)
	LocalProjectID(ctx context.Context, apID string) (string, error)
	ActorID(ctx context.Context, apID string) (*string, error)
	CreatedActivity(ctx context.Context, actorID, objectAPID string) (*createdActivity, error)
}

// Handler accepts local client-to-server ActivityPub outbox submissions.
type Handler struct {
	repo     Repository
	tickets  TicketCreator
	comments CommentCreator
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db *sqlx.DB
}

type localUser struct {
	ID       string `db:"id"`
	Username string `db:"username"`
	APID     string `db:"ap_id"`
}

type clientActivity struct {
	ID     string          `json:"id"`
	Type   any             `json:"type"`
	Actor  string          `json:"actor"`
	Object json.RawMessage `json:"object"`
	Target any             `json:"target"`
}

type createdActivity struct {
	APID     string          `db:"ap_id"`
	Document json.RawMessage `db:"document"`
}

// NewHandler creates a C2S outbox handler with the default repository.
func NewHandler(db *sqlx.DB, cfg activitypub.Config, tickets TicketCreator, comments CommentCreator) *Handler {
	return NewHandlerWithRepository(NewRepository(db), tickets, comments)
}

// NewRepository creates a PostgreSQL-backed C2S repository.
func NewRepository(db *sqlx.DB) *PgRepository {
	return &PgRepository{db: db}
}

// NewHandlerWithRepository creates a C2S outbox handler with explicit dependencies.
func NewHandlerWithRepository(repo Repository, tickets TicketCreator, comments CommentCreator) *Handler {
	return &Handler{repo: repo, tickets: tickets, comments: comments}
}

// RegisterRoutes registers client-to-server outbox posting routes.
func (h *Handler) RegisterRoutes(e *echo.Echo, middleware ...echo.MiddlewareFunc) {
	e.POST("/users/:username/outbox", h.PostUserOutbox, middleware...)
}

// PostUserOutbox accepts supported local Create activities from an authenticated user.
func (h *Handler) PostUserOutbox(c echo.Context) error {
	if !isSupportedContentType(c.Request().Header.Get(echo.HeaderContentType)) {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{"error": "content type must be application/activity+json or application/ld+json"})
	}
	if !acceptsActivityJSON(c.Request().Header.Get(echo.HeaderAccept)) {
		return c.JSON(http.StatusNotAcceptable, map[string]string{"error": "accept header must allow activitypub json"})
	}

	userID, ok := c.Get("userID").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authenticated user"})
	}

	user, err := h.repo.LocalUser(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authenticated user not found"})
	}
	if c.Param("username") != user.Username {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "outbox user does not match authenticated user"})
	}

	var activity clientActivity
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, maxOutboxBodyBytes))
	if err := decoder.Decode(&activity); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid activity json"})
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "activity json must contain one document"})
	}
	if strings.TrimSpace(activity.ID) != "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "server-assigned activity id is required"})
	}
	if !hasType(activity.Type, "Create") {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": errUnsupportedActivity.Error()})
	}
	if strings.TrimSpace(activity.Actor) == "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "activity actor is required"})
	}
	if strings.TrimSpace(activity.Actor) != user.APID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": errActorMismatch.Error()})
	}

	object, err := decodeObject(activity.Object)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if object.optionalString("id") != "" {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "server-assigned object id is required"})
	}
	if attributedTo := object.optionalString("attributedTo"); attributedTo != "" && attributedTo != user.APID {
		return c.JSON(http.StatusForbidden, map[string]string{"error": errActorMismatch.Error()})
	}

	created, err := h.handleCreate(c.Request().Context(), user, activity, object)
	if err != nil {
		return h.writeCreateError(c, err)
	}

	c.Response().Header().Set(echo.HeaderLocation, created.APID)
	return c.Blob(http.StatusCreated, activitypub.ActivityJSONMediaType, created.Document)
}

func (h *Handler) handleCreate(ctx context.Context, user *localUser, activity clientActivity, object activityObject) (*createdActivity, error) {
	switch {
	case hasType(object["type"], "Note"):
		return h.createNote(ctx, user, object)
	case hasType(object["type"], "forge:Ticket") || hasType(object["type"], "Ticket"):
		return h.createTicket(ctx, user, activity, object)
	default:
		return nil, errUnsupportedActivity
	}
}

func (h *Handler) createNote(ctx context.Context, user *localUser, object activityObject) (*createdActivity, error) {
	ticketAPID := object.optionalString("inReplyTo")
	if ticketAPID == "" {
		return nil, invalidActivity("note inReplyTo is required")
	}
	content := object.optionalString("content")
	if strings.TrimSpace(content) == "" {
		return nil, invalidActivity("note content is required")
	}
	ticketID, err := h.repo.LocalTicketID(ctx, ticketAPID)
	if err != nil {
		return nil, err
	}

	created, err := h.comments.CreateComment(ctx, ticketID, user.ID, content)
	if err != nil {
		return nil, err
	}
	return h.repo.CreatedActivity(ctx, user.ID, created.APID)
}

func (h *Handler) createTicket(ctx context.Context, user *localUser, activity clientActivity, object activityObject) (*createdActivity, error) {
	projectAPID := object.optionalString("context")
	if projectAPID == "" {
		projectAPID = firstString(activity.Target)
	}
	if projectAPID == "" {
		return nil, invalidActivity("ticket context is required")
	}
	projectID, err := h.repo.LocalProjectID(ctx, projectAPID)
	if err != nil {
		return nil, err
	}

	title := object.optionalString("name")
	if strings.TrimSpace(title) == "" {
		return nil, invalidActivity("ticket name is required")
	}

	parentID, err := h.optionalLocalTicketID(ctx, object.optionalString("inReplyTo"))
	if err != nil {
		return nil, err
	}
	assigneeID, err := h.optionalActorID(ctx, firstString(object["forge:assignedTo"]))
	if err != nil {
		return nil, err
	}

	created, err := h.tickets.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       title,
		Description: object.optionalString("content"),
		Priority:    object.optionalString("forge:priority"),
		Type:        object.optionalString("forge:ticketType"),
		ParentID:    parentID,
		AssigneeID:  assigneeID,
	}, projectID, user.ID)
	if err != nil {
		return nil, err
	}
	return h.repo.CreatedActivity(ctx, user.ID, created.APID)
}

// LocalUser loads the local actor record for an authenticated user.
func (r *PgRepository) LocalUser(ctx context.Context, userID string) (*localUser, error) {
	var user localUser
	err := r.db.GetContext(ctx, &user, `
		SELECT actor.id::text, users.username, actor.ap_id
		FROM users
		JOIN actors actor ON actor.id = users.id
		WHERE users.id = $1 AND actor.is_local = true
	`, userID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// LocalTicketID resolves a local ticket UUID from its ActivityPub ID.
func (r *PgRepository) LocalTicketID(ctx context.Context, apID string) (string, error) {
	var id string
	err := r.db.GetContext(ctx, &id, `
		SELECT ticket.id::text
		FROM tickets ticket
		WHERE ticket.ap_id = $1
	`, apID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("local ticket not found")
	}
	return id, err
}

func (h *Handler) optionalLocalTicketID(ctx context.Context, apID string) (*string, error) {
	if apID == "" {
		return nil, nil
	}
	id, err := h.repo.LocalTicketID(ctx, apID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// LocalProjectID resolves a local project UUID from its ActivityPub ID.
func (r *PgRepository) LocalProjectID(ctx context.Context, apID string) (string, error) {
	var id string
	err := r.db.GetContext(ctx, &id, `
		SELECT project.id::text
		FROM projects project
		JOIN actors actor ON actor.id = project.id
		WHERE actor.ap_id = $1 AND actor.is_local = true
	`, apID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("local project not found")
	}
	return id, err
}

func (h *Handler) optionalActorID(ctx context.Context, apID string) (*string, error) {
	if apID == "" {
		return nil, nil
	}
	return h.repo.ActorID(ctx, apID)
}

// ActorID resolves any local or known remote actor UUID from its ActivityPub ID.
func (r *PgRepository) ActorID(ctx context.Context, apID string) (*string, error) {
	var id string
	err := r.db.GetContext(ctx, &id, `
		SELECT id::text
		FROM actors
		WHERE ap_id = $1
	`, apID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("actor not found")
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// CreatedActivity loads the most recent Create activity for a created object.
func (r *PgRepository) CreatedActivity(ctx context.Context, actorID, objectAPID string) (*createdActivity, error) {
	var activity createdActivity
	err := r.db.GetContext(ctx, &activity, `
		SELECT ap_id, document
		FROM ap_activities
		WHERE activity_type = 'Create'
			AND actor_id = $1
			AND object_ap_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, actorID, objectAPID)
	if err != nil {
		return nil, err
	}
	return &activity, nil
}

func (h *Handler) writeCreateError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, errUnsupportedActivity):
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": errUnsupportedActivity.Error()})
	case errors.Is(err, errInvalidActivity):
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": strings.TrimPrefix(err.Error(), errInvalidActivity.Error()+": ")})
	case strings.Contains(err.Error(), "not found"):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "access denied"), strings.Contains(err.Error(), "insufficient permissions"):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
}

func invalidActivity(message string) error {
	return fmt.Errorf("%w: %s", errInvalidActivity, message)
}

type activityObject map[string]any

func decodeObject(raw json.RawMessage) (activityObject, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("activity object is required")
	}
	var object activityObject
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.New("activity object must be an object")
	}
	if len(object) == 0 {
		return nil, errors.New("activity object is required")
	}
	return object, nil
}

func (o activityObject) optionalString(key string) string {
	return firstString(o[key])
}

func hasType(value any, expected string) bool {
	expected = strings.ToLower(expected)
	for _, item := range stringValues(value) {
		if strings.ToLower(item) == expected {
			return true
		}
	}
	return false
}

func isSupportedContentType(header string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(header, ";")[0])
	}
	return isSupportedActivityMediaType(mediaType)
}

func acceptsActivityJSON(header string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			mediaType = strings.TrimSpace(strings.Split(part, ";")[0])
		}
		mediaType = strings.ToLower(mediaType)
		if mediaType == "*/*" || mediaType == "application/*" || mediaType == "application/json" || isSupportedActivityMediaType(mediaType) {
			return true
		}
	}
	return false
}

func isSupportedActivityMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == activitypub.ActivityJSONMediaType || mediaType == "application/ld+json"
}

func firstString(value any) string {
	values := stringValues(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
		return values
	default:
		return nil
	}
}

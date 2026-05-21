package activitypub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
)

const (
	defaultCollectionPageLimit = 20
	maxCollectionPageLimit     = 100
)

// Handler exposes local ActivityPub read endpoints for actors, objects, and collections.
type Handler struct {
	db         *sqlx.DB
	cfg        Config
	authorizer Authorizer
}

// Authorizer checks access to protected local ActivityPub resources.
type Authorizer interface {
	AuthorizeActor(ctx context.Context, req *http.Request, actorID string) error
	AuthorizeProject(ctx context.Context, req *http.Request, projectID string) error
}

// collectionPageRequest carries ActivityPub collection pagination flags.
type collectionPageRequest struct {
	Page   bool
	Limit  int
	Offset int
}

// NewHandler creates an ActivityPub read handler without access enforcement.
func NewHandler(db *sqlx.DB, cfg Config) *Handler {
	return &Handler{db: db, cfg: cfg}
}

// NewHandlerWithAuthorizer creates an ActivityPub read handler with access enforcement.
func NewHandlerWithAuthorizer(db *sqlx.DB, cfg Config, authorizer Authorizer) *Handler {
	return &Handler{db: db, cfg: cfg, authorizer: authorizer}
}

// RegisterRoutes registers ActivityPub actor, object, activity, and collection routes.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/users/:username", h.GetUserActor, requireActivityPubAccept)
	e.GET("/users/:username/inbox", h.UserInbox, requireActivityPubAccept)
	e.GET("/users/:username/outbox", h.UserOutbox, requireActivityPubAccept)
	e.GET("/users/:username/followers", h.UserFollowers, requireActivityPubAccept)
	e.GET("/projects/:id", h.GetProjectActor, requireActivityPubAccept)
	e.GET("/projects/:id/inbox", h.ProjectInbox, requireActivityPubAccept)
	e.GET("/projects/:id/outbox", h.ProjectOutbox, requireActivityPubAccept)
	e.GET("/projects/:id/followers", h.ProjectFollowers, requireActivityPubAccept)
	e.GET("/projects/:id/tickets", h.ProjectTickets, requireActivityPubAccept)
	e.GET("/tickets/:id", h.GetTicket, requireActivityPubAccept)
	e.GET("/comments/:id", h.GetComment, requireActivityPubAccept)
	e.GET("/activities/:id", h.GetActivity, requireActivityPubAccept)
}

// GetUserActor returns a local Person actor document.
func (h *Handler) GetUserActor(c echo.Context) error {
	apID := UserAPID(h.cfg, c.Param("username"))
	return h.writeObject(c, apID)
}

// GetProjectActor returns a local project Group actor document.
func (h *Handler) GetProjectActor(c echo.Context) error {
	apID := ProjectAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

// GetTicket returns a local ForgeFed ticket document.
func (h *Handler) GetTicket(c echo.Context) error {
	apID := TicketAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

// GetComment returns a local Note document.
func (h *Handler) GetComment(c echo.Context) error {
	apID := CommentAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

// GetActivity returns a stored ActivityStreams activity document.
func (h *Handler) GetActivity(c echo.Context) error {
	apID := ActivityAPID(h.cfg, c.Param("id"))
	var activity struct {
		Document  json.RawMessage `db:"document"`
		ProjectID sql.NullString  `db:"project_id"`
	}
	if err := h.db.GetContext(c.Request().Context(), &activity, `
		SELECT
			activity.document,
			COALESCE(
				target_project_actor.id::text,
				object_project_actor.id::text,
				activity_project_actor.id::text,
				object_ticket.project_id::text,
				target_ticket.project_id::text,
				object_comment_ticket.project_id::text,
				target_comment_ticket.project_id::text,
				object_invite.project_id::text,
				target_invite.project_id::text
			) AS project_id
		FROM ap_activities activity
		LEFT JOIN actors target_project_actor
			ON target_project_actor.ap_id = activity.target_ap_id
			AND target_project_actor.type = 'Group'
			AND target_project_actor.is_local = true
		LEFT JOIN actors object_project_actor
			ON object_project_actor.ap_id = activity.object_ap_id
			AND object_project_actor.type = 'Group'
			AND object_project_actor.is_local = true
		LEFT JOIN actors activity_project_actor
			ON activity_project_actor.id = activity.actor_id
			AND activity_project_actor.type = 'Group'
			AND activity_project_actor.is_local = true
		LEFT JOIN ap_objects object_scope ON object_scope.ap_id = activity.object_ap_id
		LEFT JOIN tickets object_ticket
			ON object_scope.local_ref_table = 'tickets'
			AND object_ticket.id = object_scope.local_ref_id
		LEFT JOIN comments object_comment
			ON object_scope.local_ref_table = 'comments'
			AND object_comment.id = object_scope.local_ref_id
		LEFT JOIN tickets object_comment_ticket ON object_comment_ticket.id = object_comment.ticket_id
		LEFT JOIN ap_objects target_scope ON target_scope.ap_id = activity.target_ap_id
		LEFT JOIN tickets target_ticket
			ON target_scope.local_ref_table = 'tickets'
			AND target_ticket.id = target_scope.local_ref_id
		LEFT JOIN comments target_comment
			ON target_scope.local_ref_table = 'comments'
			AND target_comment.id = target_scope.local_ref_id
		LEFT JOIN tickets target_comment_ticket ON target_comment_ticket.id = target_comment.ticket_id
		LEFT JOIN project_invites object_invite ON object_invite.ap_id = activity.object_ap_id
		LEFT JOIN project_invites target_invite ON target_invite.ap_id = activity.target_ap_id
		WHERE activity.ap_id = $1
	`, apID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "activity not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load activity"})
	}
	if activity.ProjectID.Valid {
		if err := h.requireProjectAccess(c, activity.ProjectID.String); err != nil {
			return err
		}
	}
	return c.Blob(http.StatusOK, ActivityJSONMediaType, activity.Document)
}

// UserInbox returns the inbox collection for a local user actor.
func (h *Handler) UserInbox(c echo.Context) error {
	return h.actorActivityCollection(c, UserAPID(h.cfg, c.Param("username")), "inbox")
}

// UserOutbox returns the outbox collection for a local user actor.
func (h *Handler) UserOutbox(c echo.Context) error {
	return h.actorActivityCollection(c, UserAPID(h.cfg, c.Param("username")), "outbox")
}

// UserFollowers returns the followers collection for a local user actor.
func (h *Handler) UserFollowers(c echo.Context) error {
	return h.followersCollection(c, UserAPID(h.cfg, c.Param("username")))
}

// ProjectInbox returns the inbox collection for a local project actor.
func (h *Handler) ProjectInbox(c echo.Context) error {
	return h.actorActivityCollection(c, ProjectAPID(h.cfg, c.Param("id")), "inbox")
}

// ProjectOutbox returns the outbox collection for a local project actor.
func (h *Handler) ProjectOutbox(c echo.Context) error {
	return h.actorActivityCollection(c, ProjectAPID(h.cfg, c.Param("id")), "outbox")
}

// ProjectFollowers returns the followers collection for a local project actor.
func (h *Handler) ProjectFollowers(c echo.Context) error {
	return h.followersCollection(c, ProjectAPID(h.cfg, c.Param("id")))
}

// ProjectTickets returns the ticket collection for a local project actor.
func (h *Handler) ProjectTickets(c echo.Context) error {
	return h.projectTicketsCollection(c, ProjectAPID(h.cfg, c.Param("id")))
}

// writeObject loads a stored ActivityPub object and writes its JSON-LD document.
func (h *Handler) writeObject(c echo.Context, apID string) error {
	var object struct {
		Document      json.RawMessage `db:"document"`
		LocalRefTable sql.NullString  `db:"local_ref_table"`
		LocalRefID    sql.NullString  `db:"local_ref_id"`
	}
	if err := h.db.GetContext(c.Request().Context(), &object, `
		SELECT document, local_ref_table, local_ref_id::text AS local_ref_id
		FROM ap_objects
		WHERE ap_id = $1
	`, apID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "object not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load object"})
	}
	if err := h.authorizeObject(c, object.LocalRefTable, object.LocalRefID); err != nil {
		return err
	}
	return c.Blob(http.StatusOK, ActivityJSONMediaType, object.Document)
}

// actorActivityCollection renders an actor inbox or outbox as an ordered collection.
func (h *Handler) actorActivityCollection(c echo.Context, actorAPID, box string) error {
	page, err := parseCollectionPageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var actor struct {
		ID      string `db:"id"`
		Type    string `db:"type"`
		IsLocal bool   `db:"is_local"`
	}
	if err := h.db.GetContext(c.Request().Context(), &actor, `
		SELECT id::text, type, is_local FROM actors WHERE ap_id = $1
	`, actorAPID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "actor not found"})
	}
	if err := h.authorizeActorCollection(c, actor.ID, actor.Type, actor.IsLocal); err != nil {
		return err
	}

	table := "actor_inbox_items"
	collectionID := Inbox(actorAPID)
	orderColumn := "received_at"
	if box == "outbox" {
		table = "actor_outbox_items"
		collectionID = Outbox(actorAPID)
		orderColumn = "published_at"
	}

	var totalItems int
	if err := h.db.GetContext(c.Request().Context(), &totalItems, `
		SELECT count(*)
		FROM `+table+`
		WHERE actor_id = $1
	`, actor.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load collection"})
	}
	if !page.Page {
		return writeActivityJSON(c, http.StatusOK, OrderedCollectionDocument(
			collectionID,
			totalItems,
			collectionPageURL(collectionID, page.Limit, 0),
		))
	}

	query := `
		SELECT a.document
		FROM ` + table + ` i
		JOIN ap_activities a ON a.id = i.activity_id
		WHERE i.actor_id = $1
		ORDER BY i.` + orderColumn + ` DESC, i.activity_id DESC
		LIMIT $2 OFFSET $3
	`
	var raws []json.RawMessage
	if err := h.db.SelectContext(c.Request().Context(), &raws, query, actor.ID, page.Limit, page.Offset); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load collection"})
	}
	items, err := decodeRawItems(raws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to decode collection"})
	}

	next, prev := collectionPageLinks(collectionID, totalItems, page.Limit, page.Offset)
	return writeActivityJSON(c, http.StatusOK, OrderedCollectionPageDocument(
		collectionPageURL(collectionID, page.Limit, page.Offset),
		collectionID,
		totalItems,
		items,
		next,
		prev,
	))
}

// followersCollection renders the accepted followers of a local actor.
func (h *Handler) followersCollection(c echo.Context, actorAPID string) error {
	page, err := parseCollectionPageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var actor struct {
		ID      string `db:"id"`
		Type    string `db:"type"`
		IsLocal bool   `db:"is_local"`
	}
	if err := h.db.GetContext(c.Request().Context(), &actor, `
		SELECT id::text, type, is_local FROM actors WHERE ap_id = $1
	`, actorAPID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "actor not found"})
	}
	if err := h.authorizeActorCollection(c, actor.ID, actor.Type, actor.IsLocal); err != nil {
		return err
	}

	collectionID := Followers(actorAPID)
	var totalItems int
	if err := h.db.GetContext(c.Request().Context(), &totalItems, `
		SELECT count(*)
		FROM actor_follows f
		WHERE f.followed_actor_id = $1 AND f.state = 'accepted'
	`, actor.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load followers"})
	}
	if !page.Page {
		return writeActivityJSON(c, http.StatusOK, OrderedCollectionDocument(
			collectionID,
			totalItems,
			collectionPageURL(collectionID, page.Limit, 0),
		))
	}

	var followerAPIDs []string
	if err := h.db.SelectContext(c.Request().Context(), &followerAPIDs, `
		SELECT follower.ap_id
		FROM actor_follows f
		JOIN actors follower ON follower.id = f.follower_actor_id
		WHERE f.followed_actor_id = $1 AND f.state = 'accepted'
		ORDER BY f.created_at ASC, follower.ap_id ASC
		LIMIT $2 OFFSET $3
	`, actor.ID, page.Limit, page.Offset); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load followers"})
	}
	next, prev := collectionPageLinks(collectionID, totalItems, page.Limit, page.Offset)
	return writeActivityJSON(c, http.StatusOK, OrderedCollectionPageDocument(
		collectionPageURL(collectionID, page.Limit, page.Offset),
		collectionID,
		totalItems,
		stringItems(followerAPIDs),
		next,
		prev,
	))
}

// projectTicketsCollection renders the ticket collection for a local project actor.
func (h *Handler) projectTicketsCollection(c echo.Context, projectAPID string) error {
	page, err := parseCollectionPageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var projectID string
	if err := h.db.GetContext(c.Request().Context(), &projectID, `
		SELECT project.id::text
		FROM projects project
		JOIN actors actor ON actor.id = project.id
		WHERE actor.ap_id = $1
	`, projectAPID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load project"})
	}
	if err := h.requireProjectAccess(c, projectID); err != nil {
		return err
	}

	collectionID := ProjectTickets(projectAPID)
	var totalItems int
	if err := h.db.GetContext(c.Request().Context(), &totalItems, `
		SELECT count(*)
		FROM tickets
		WHERE project_id = $1
	`, projectID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load project tickets"})
	}
	if !page.Page {
		return writeActivityJSON(c, http.StatusOK, OrderedCollectionDocument(
			collectionID,
			totalItems,
			collectionPageURL(collectionID, page.Limit, 0),
		))
	}

	var ticketAPIDs []string
	if err := h.db.SelectContext(c.Request().Context(), &ticketAPIDs, `
		SELECT ap_id
		FROM tickets
		WHERE project_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, projectID, page.Limit, page.Offset); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load project tickets"})
	}

	next, prev := collectionPageLinks(collectionID, totalItems, page.Limit, page.Offset)
	return writeActivityJSON(c, http.StatusOK, OrderedCollectionPageDocument(
		collectionPageURL(collectionID, page.Limit, page.Offset),
		collectionID,
		totalItems,
		stringItems(ticketAPIDs),
		next,
		prev,
	))
}

// authorizeObject enforces project visibility for ticket and comment documents.
func (h *Handler) authorizeObject(c echo.Context, localRefTable, localRefID sql.NullString) error {
	if h.authorizer == nil || !localRefTable.Valid || !localRefID.Valid {
		return nil
	}

	var projectID string
	switch localRefTable.String {
	case "tickets":
		if err := h.db.GetContext(c.Request().Context(), &projectID, `
			SELECT project_id::text
			FROM tickets
			WHERE id = $1
		`, localRefID.String); err != nil {
			return h.objectScopeError(c, err)
		}
	case "comments":
		if err := h.db.GetContext(c.Request().Context(), &projectID, `
			SELECT ticket.project_id::text
			FROM comments comment
			JOIN tickets ticket ON ticket.id = comment.ticket_id
			WHERE comment.id = $1
		`, localRefID.String); err != nil {
			return h.objectScopeError(c, err)
		}
	default:
		return nil
	}

	return h.requireProjectAccess(c, projectID)
}

// objectScopeError maps missing or failed object-scope lookups to HTTP responses.
func (h *Handler) objectScopeError(c echo.Context, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "object not found"})
	}
	return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load object scope"})
}

// authorizeActorCollection chooses actor or project authorization for collections.
func (h *Handler) authorizeActorCollection(c echo.Context, actorID, actorType string, isLocal bool) error {
	if h.authorizer == nil {
		return nil
	}
	if actorType == "Group" && isLocal {
		return h.requireProjectAccess(c, actorID)
	}
	if isLocal {
		return h.requireActorAccess(c, actorID)
	}
	return nil
}

// requireActorAccess checks whether the current request may read an actor collection.
func (h *Handler) requireActorAccess(c echo.Context, actorID string) error {
	if h.authorizer == nil {
		return nil
	}
	if err := h.authorizer.AuthorizeActor(c.Request().Context(), c.Request(), actorID); err != nil {
		return h.authorizationError(c, err)
	}
	return nil
}

// requireProjectAccess checks whether the current request may read project-scoped data.
func (h *Handler) requireProjectAccess(c echo.Context, projectID string) error {
	if h.authorizer == nil {
		return nil
	}
	if err := h.authorizer.AuthorizeProject(c.Request().Context(), c.Request(), projectID); err != nil {
		return h.authorizationError(c, err)
	}
	return nil
}

// authorizationError converts authorization failures into stable HTTP responses.
func (h *Handler) authorizationError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrMissingAuthorization), errors.Is(err, ErrInvalidAuthorization):
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrAccessDenied):
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to authorize activitypub request"})
	}
}

// parseCollectionPageRequest parses bounded ActivityPub collection pagination.
func parseCollectionPageRequest(c echo.Context) (collectionPageRequest, error) {
	request := collectionPageRequest{Limit: defaultCollectionPageLimit}
	pageParam := strings.ToLower(strings.TrimSpace(c.QueryParam("page")))
	switch pageParam {
	case "", "false", "0":
	case "true", "1":
		request.Page = true
	default:
		return request, fmt.Errorf("invalid page parameter")
	}

	if limitParam := strings.TrimSpace(c.QueryParam("limit")); limitParam != "" {
		limit, err := strconv.Atoi(limitParam)
		if err != nil || limit < 1 {
			return request, fmt.Errorf("invalid limit parameter")
		}
		if limit > maxCollectionPageLimit {
			limit = maxCollectionPageLimit
		}
		request.Limit = limit
	}

	if offsetParam := strings.TrimSpace(c.QueryParam("offset")); offsetParam != "" {
		offset, err := strconv.Atoi(offsetParam)
		if err != nil || offset < 0 {
			return request, fmt.Errorf("invalid offset parameter")
		}
		request.Offset = offset
	}

	return request, nil
}

// collectionPageURL builds the canonical URL for one ordered collection page.
func collectionPageURL(collectionID string, limit, offset int) string {
	values := url.Values{}
	values.Set("page", "true")
	values.Set("limit", strconv.Itoa(limit))
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	return collectionID + "?" + values.Encode()
}

// collectionPageLinks returns next and previous page URLs for an ordered collection.
func collectionPageLinks(collectionID string, totalItems, limit, offset int) (next string, prev string) {
	if offset+limit < totalItems {
		next = collectionPageURL(collectionID, limit, offset+limit)
	}
	if offset > 0 {
		previousOffset := offset - limit
		if previousOffset < 0 {
			previousOffset = 0
		}
		prev = collectionPageURL(collectionID, limit, previousOffset)
	}
	return next, prev
}

// writeActivityJSON writes an ActivityPub JSON document with the canonical media type.
func writeActivityJSON(c echo.Context, status int, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode activitypub document"})
	}
	return c.Blob(status, ActivityJSONMediaType, raw)
}

// requireActivityPubAccept rejects reads that cannot accept ActivityPub JSON.
func requireActivityPubAccept(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !acceptsActivityPubResponse(c.Request().Header.Get(echo.HeaderAccept)) {
			return c.JSON(http.StatusNotAcceptable, map[string]string{"error": "accept header must allow activitypub json"})
		}
		return next(c)
	}
}

// acceptsActivityPubResponse reports whether an Accept header allows ActivityPub JSON.
func acceptsActivityPubResponse(header string) bool {
	return acceptsMediaType(header, isActivityPubResponseMediaType)
}

// acceptsMediaType evaluates comma-separated Accept header entries with q-values.
func acceptsMediaType(header string, allowed func(string) bool) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			mediaType = strings.TrimSpace(strings.Split(part, ";")[0])
			params = nil
		}
		if isZeroQuality(params["q"]) {
			continue
		}
		if allowed(strings.ToLower(mediaType)) {
			return true
		}
	}
	return false
}

// isActivityPubResponseMediaType recognizes JSON media types usable for ActivityPub reads.
func isActivityPubResponseMediaType(mediaType string) bool {
	return mediaType == "*/*" ||
		mediaType == "application/*" ||
		mediaType == "application/json" ||
		mediaType == ActivityJSONMediaType ||
		mediaType == "application/ld+json"
}

// isZeroQuality reports whether an Accept q-value explicitly disables a media type.
func isZeroQuality(raw string) bool {
	if raw == "" {
		return false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return err == nil && value <= 0
}

// stringItems converts URI strings into ActivityStreams collection item values.
func stringItems(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}

// decodeRawItems decodes stored JSON activity documents for collection pages.
func decodeRawItems(raws []json.RawMessage) ([]any, error) {
	items := make([]any, 0, len(raws))
	for _, raw := range raws {
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

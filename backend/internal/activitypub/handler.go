package activitypub

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

type Handler struct {
	db  *sqlx.DB
	cfg Config
}

type collectionPageRequest struct {
	Page   bool
	Limit  int
	Offset int
}

func NewHandler(db *sqlx.DB, cfg Config) *Handler {
	return &Handler{db: db, cfg: cfg}
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/users/:username", h.GetUserActor)
	e.GET("/users/:username/inbox", h.UserInbox)
	e.GET("/users/:username/outbox", h.UserOutbox)
	e.GET("/users/:username/followers", h.UserFollowers)
	e.GET("/projects/:id", h.GetProjectActor)
	e.GET("/projects/:id/inbox", h.ProjectInbox)
	e.GET("/projects/:id/outbox", h.ProjectOutbox)
	e.GET("/projects/:id/followers", h.ProjectFollowers)
	e.GET("/tickets/:id", h.GetTicket)
	e.GET("/comments/:id", h.GetComment)
	e.GET("/activities/:id", h.GetActivity)
}

func (h *Handler) GetUserActor(c echo.Context) error {
	apID := UserAPID(h.cfg, c.Param("username"))
	return h.writeObject(c, apID)
}

func (h *Handler) GetProjectActor(c echo.Context) error {
	apID := ProjectAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

func (h *Handler) GetTicket(c echo.Context) error {
	apID := TicketAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

func (h *Handler) GetComment(c echo.Context) error {
	apID := CommentAPID(h.cfg, c.Param("id"))
	return h.writeObject(c, apID)
}

func (h *Handler) GetActivity(c echo.Context) error {
	apID := ActivityAPID(h.cfg, c.Param("id"))
	var raw json.RawMessage
	if err := h.db.GetContext(c.Request().Context(), &raw, `
		SELECT document
		FROM ap_activities
		WHERE ap_id = $1
	`, apID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "activity not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load activity"})
	}
	return c.Blob(http.StatusOK, ActivityJSONMediaType, raw)
}

func (h *Handler) UserInbox(c echo.Context) error {
	return h.actorActivityCollection(c, UserAPID(h.cfg, c.Param("username")), "inbox")
}

func (h *Handler) UserOutbox(c echo.Context) error {
	return h.actorActivityCollection(c, UserAPID(h.cfg, c.Param("username")), "outbox")
}

func (h *Handler) UserFollowers(c echo.Context) error {
	return h.followersCollection(c, UserAPID(h.cfg, c.Param("username")))
}

func (h *Handler) ProjectInbox(c echo.Context) error {
	return h.actorActivityCollection(c, ProjectAPID(h.cfg, c.Param("id")), "inbox")
}

func (h *Handler) ProjectOutbox(c echo.Context) error {
	return h.actorActivityCollection(c, ProjectAPID(h.cfg, c.Param("id")), "outbox")
}

func (h *Handler) ProjectFollowers(c echo.Context) error {
	return h.followersCollection(c, ProjectAPID(h.cfg, c.Param("id")))
}

func (h *Handler) writeObject(c echo.Context, apID string) error {
	var raw json.RawMessage
	if err := h.db.GetContext(c.Request().Context(), &raw, `
		SELECT document
		FROM ap_objects
		WHERE ap_id = $1
	`, apID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "object not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load object"})
	}
	return c.Blob(http.StatusOK, ActivityJSONMediaType, raw)
}

func (h *Handler) actorActivityCollection(c echo.Context, actorAPID, box string) error {
	page, err := parseCollectionPageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var actorID string
	if err := h.db.GetContext(c.Request().Context(), &actorID, `
		SELECT id::text FROM actors WHERE ap_id = $1
	`, actorAPID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "actor not found"})
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
	`, actorID); err != nil {
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
	if err := h.db.SelectContext(c.Request().Context(), &raws, query, actorID, page.Limit, page.Offset); err != nil {
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

func (h *Handler) followersCollection(c echo.Context, actorAPID string) error {
	page, err := parseCollectionPageRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	var actorID string
	if err := h.db.GetContext(c.Request().Context(), &actorID, `
		SELECT id::text FROM actors WHERE ap_id = $1
	`, actorAPID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "actor not found"})
	}

	collectionID := Followers(actorAPID)
	var totalItems int
	if err := h.db.GetContext(c.Request().Context(), &totalItems, `
		SELECT count(*)
		FROM actor_follows f
		WHERE f.followed_actor_id = $1 AND f.state = 'accepted'
	`, actorID); err != nil {
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
	`, actorID, page.Limit, page.Offset); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load followers"})
	}
	items := make([]any, 0, len(followerAPIDs))
	for _, followerAPID := range followerAPIDs {
		items = append(items, followerAPID)
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

func collectionPageURL(collectionID string, limit, offset int) string {
	values := url.Values{}
	values.Set("page", "true")
	values.Set("limit", strconv.Itoa(limit))
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	return collectionID + "?" + values.Encode()
}

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

func writeActivityJSON(c echo.Context, status int, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to encode activitypub document"})
	}
	return c.Blob(status, ActivityJSONMediaType, raw)
}

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

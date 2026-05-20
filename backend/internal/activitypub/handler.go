package activitypub

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	db  *sqlx.DB
	cfg Config
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
	return c.JSONBlob(http.StatusOK, raw)
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
		WHERE ap_id = $1 AND is_deleted = false
	`, apID); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "object not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load object"})
	}
	return c.JSONBlob(http.StatusOK, raw)
}

func (h *Handler) actorActivityCollection(c echo.Context, actorAPID, box string) error {
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

	query := `
		SELECT a.document
		FROM ` + table + ` i
		JOIN ap_activities a ON a.id = i.activity_id
		WHERE i.actor_id = $1
		ORDER BY i.` + orderColumn + ` DESC
	`
	var raws []json.RawMessage
	if err := h.db.SelectContext(c.Request().Context(), &raws, query, actorID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load collection"})
	}
	items, err := decodeRawItems(raws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to decode collection"})
	}
	return c.JSON(http.StatusOK, CollectionDocument(collectionID, "OrderedCollection", items))
}

func (h *Handler) followersCollection(c echo.Context, actorAPID string) error {
	var actorID string
	if err := h.db.GetContext(c.Request().Context(), &actorID, `
		SELECT id::text FROM actors WHERE ap_id = $1
	`, actorAPID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "actor not found"})
	}

	var raws []json.RawMessage
	if err := h.db.SelectContext(c.Request().Context(), &raws, `
		SELECT o.document
		FROM actor_follows f
		JOIN ap_objects o ON o.actor_id = f.follower_actor_id
		WHERE f.followed_actor_id = $1 AND f.state = 'accepted'
		ORDER BY f.created_at ASC
	`, actorID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load followers"})
	}
	items, err := decodeRawItems(raws)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to decode followers"})
	}
	return c.JSON(http.StatusOK, CollectionDocument(Followers(actorAPID), "Collection", items))
}

func decodeRawItems(raws []json.RawMessage) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(raws))
	for _, raw := range raws {
		var item map[string]any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

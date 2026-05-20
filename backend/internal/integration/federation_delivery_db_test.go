//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationDiscoveryAndDeliverySmoke(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
	deliveryRepo := delivery.NewRecipientRepository(db)
	deliveryService := delivery.NewService(deliveryRepo, delivery.NoopQueue{})
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)

	owner, err := userService.RegisterUser(ctx, "federation-owner", "federation-owner@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "Federation Smoke Board", "", owner.ID)
	require.NoError(t, err)

	remotePublicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)
	client := newFederationSmokeClient(t, db, remotePublicKey)
	remoteActorService := remoteactor.NewService(
		remoteactor.NewRepository(db),
		remoteactor.WithHTTPClient(client),
		remoteactor.WithWebFingerScheme("https"),
	)

	remoteActor, err := remoteActorService.Discover(ctx, "acct:reviewer@remote.example")
	require.NoError(t, err)
	assert.Equal(t, "https://remote.example/users/reviewer", remoteActor.APID)
	assert.Equal(t, "reviewer@remote.example", remoteActor.Handle)
	assert.Equal(t, 1, client.webFingerRequests)
	assert.Equal(t, 1, client.actorFetches)

	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
	`, remoteActor.ID, createdProject.ID)
	require.NoError(t, err)

	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Verify federation delivery",
		Description: "Smoke test remote discovery, signing, delivery, and retry state.",
		Priority:    "high",
		Type:        "task",
	}, createdProject.ID, owner.ID)
	require.NoError(t, err)

	deliveredID := requireDeliveryIDForTarget(t, db, createdTicket.APID, remoteActor.InboxURL)
	worker := delivery.NewWorker(
		delivery.NewRepository(db),
		httpsig.NewService(httpsig.NewRepository(db)),
		client,
		delivery.WithRemoteActorRefresher(remoteActorService),
		delivery.WithTargetActorRefreshMaxAge(0),
	)

	err = worker.HandleDeliveryTask(ctx, federationDeliveryTask(t, deliveredID))
	require.NoError(t, err)

	assert.Equal(t, 2, client.actorFetches)
	assert.Equal(t, 1, client.deliveries)
	assert.Equal(t, owner.ID, client.verifiedActorID)
	assert.Equal(t, owner.APID, client.verifiedActorAPID)
	assert.Equal(t, activitypub.ActivityJSONMediaType, client.receivedContentType)
	assert.Equal(t, activitypub.ActivityJSONMediaType, client.receivedAccept)
	assert.Equal(t, delivery.StateDelivered, requireDeliveryState(t, db, deliveredID).State)

	var deliveredActivity map[string]any
	require.NoError(t, json.Unmarshal(client.receivedBody, &deliveredActivity))
	assert.Equal(t, "Create", deliveredActivity["type"])
	assert.Equal(t, owner.APID, deliveredActivity["actor"])
	assert.Equal(t, createdTicket.APID, deliveredActivity["object"])
	assert.Equal(t, createdProject.APID, deliveredActivity["target"])

	createActivityID := requireActivityIDForObject(t, db, createdTicket.APID)
	failedDelivery, isNew, err := delivery.NewRepository(db).Create(ctx, createActivityID, "https://remote.example/users/unavailable/inbox", 3)
	require.NoError(t, err)
	require.True(t, isNew)

	err = worker.HandleDeliveryTask(ctx, federationDeliveryTask(t, failedDelivery.ID))
	require.Error(t, err)
	assert.False(t, errors.Is(err, asynq.SkipRetry))

	failedState := requireDeliveryState(t, db, failedDelivery.ID)
	assert.Equal(t, delivery.StateFailed, failedState.State)
	assert.Equal(t, 1, failedState.Attempts)
	require.True(t, failedState.NextAttemptAt.Valid)
	assert.WithinDuration(t, time.Now().UTC().Add(time.Minute), failedState.NextAttemptAt.Time, 5*time.Second)
	require.True(t, failedState.LastError.Valid)
	assert.Contains(t, failedState.LastError.String, "503")
}

type federationSmokeClient struct {
	t                   *testing.T
	verifier            *httpsig.Service
	remotePublicKey     string
	webFingerRequests   int
	actorFetches        int
	deliveries          int
	verifiedActorID     string
	verifiedActorAPID   string
	receivedBody        []byte
	receivedContentType string
	receivedAccept      string
}

func newFederationSmokeClient(t *testing.T, db *sqlx.DB, remotePublicKey string) *federationSmokeClient {
	t.Helper()

	return &federationSmokeClient{
		t:               t,
		verifier:        httpsig.NewService(httpsig.NewRepository(db)),
		remotePublicKey: remotePublicKey,
	}
}

func (c *federationSmokeClient) Do(req *http.Request) (*http.Response, error) {
	switch {
	case req.Method == http.MethodGet && req.URL.Host == "remote.example" && req.URL.Path == "/.well-known/webfinger":
		c.webFingerRequests++
		assert.Equal(c.t, "application/jrd+json", req.Header.Get("Accept"))
		assert.Equal(c.t, "acct:reviewer@remote.example", req.URL.Query().Get("resource"))
		return federationJSONResponse(http.StatusOK, map[string]any{
			"subject": "acct:reviewer@remote.example",
			"links": []map[string]any{
				{
					"rel":  "self",
					"type": activitypub.ActivityJSONMediaType,
					"href": "https://remote.example/users/reviewer",
				},
			},
		}), nil
	case req.Method == http.MethodGet && req.URL.Host == "remote.example" && req.URL.Path == "/users/reviewer":
		c.actorFetches++
		return federationJSONResponse(http.StatusOK, activitypub.ActorDocument(
			"Person",
			"https://remote.example/users/reviewer",
			"reviewer",
			"Reviewer",
			"",
			c.remotePublicKey,
		)), nil
	case req.Method == http.MethodPost && req.URL.Host == "remote.example" && req.URL.Path == "/users/reviewer/inbox":
		body, err := io.ReadAll(req.Body)
		require.NoError(c.t, err)
		verified, err := c.verifier.VerifyRequest(context.Background(), req, body)
		require.NoError(c.t, err)

		c.deliveries++
		c.verifiedActorID = verified.ActorID
		c.verifiedActorAPID = verified.ActorAPID
		c.receivedBody = body
		c.receivedContentType = req.Header.Get("Content-Type")
		c.receivedAccept = req.Header.Get("Accept")
		return federationTextResponse(http.StatusAccepted, ""), nil
	case req.Method == http.MethodPost && req.URL.Host == "remote.example" && req.URL.Path == "/users/unavailable/inbox":
		return federationTextResponse(http.StatusServiceUnavailable, "try again later"), nil
	default:
		return federationTextResponse(http.StatusNotFound, "not found"), nil
	}
}

func federationJSONResponse(status int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	resp := federationTextResponse(status, string(raw))
	resp.Header.Set("Content-Type", "application/activity+json")
	return resp
}

func federationTextResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func federationDeliveryTask(t *testing.T, deliveryID string) *asynq.Task {
	t.Helper()

	payload, err := json.Marshal(delivery.TaskPayload{DeliveryID: deliveryID})
	require.NoError(t, err)
	return asynq.NewTask(delivery.TaskDeliver, payload)
}

func requireDeliveryIDForTarget(t *testing.T, db *sqlx.DB, objectAPID, inboxURL string) string {
	t.Helper()

	var deliveryID string
	require.NoError(t, db.Get(&deliveryID, `
		SELECT delivery.id::text
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = 'Create'
			AND activity.object_ap_id = $1
			AND delivery.target_inbox_url = $2
		LIMIT 1
	`, objectAPID, inboxURL))
	return deliveryID
}

func requireActivityIDForObject(t *testing.T, db *sqlx.DB, objectAPID string) string {
	t.Helper()

	var activityID string
	require.NoError(t, db.Get(&activityID, `
		SELECT id::text
		FROM ap_activities
		WHERE activity_type = 'Create' AND object_ap_id = $1
		LIMIT 1
	`, objectAPID))
	return activityID
}

type deliveryStateRow struct {
	State         string         `db:"state"`
	Attempts      int            `db:"attempts"`
	NextAttemptAt sql.NullTime   `db:"next_attempt_at"`
	LastError     sql.NullString `db:"last_error"`
	DeliveredAt   sql.NullTime   `db:"delivered_at"`
}

func requireDeliveryState(t *testing.T, db *sqlx.DB, deliveryID string) deliveryStateRow {
	t.Helper()

	var row deliveryStateRow
	require.NoError(t, db.Get(&row, `
		SELECT state, attempts, next_attempt_at, last_error, delivered_at
		FROM activity_deliveries
		WHERE id = $1
	`, deliveryID))
	if row.State == delivery.StateDelivered {
		require.True(t, row.DeliveredAt.Valid)
	}
	if row.State == delivery.StateFailed {
		require.False(t, row.DeliveredAt.Valid)
	}
	return row
}

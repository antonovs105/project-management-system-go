//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/c2s"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/antonovs105/project-management-system-go/internal/webfinger"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalTwoInstanceFederationSmoke(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests against a migrated PostgreSQL database")
	}

	ctx := context.Background()
	router := &localFederationRouter{handlers: make(map[string]http.Handler)}
	alpha := newLocalFederationInstance(t, ctx, source, "alpha", "alpha.local.test", router)
	beta := newLocalFederationInstance(t, ctx, source, "beta", "beta.local.test", router)
	router.handlers[alpha.domain] = alpha.echo
	router.handlers[beta.domain] = beta.echo

	alice, err := alpha.userService.RegisterUser(ctx, "alice", "alice@alpha.local.test", "password123")
	require.NoError(t, err)
	bob, err := beta.userService.RegisterUser(ctx, "bob", "bob@beta.local.test", "password123")
	require.NoError(t, err)
	betaProject, err := beta.projectService.CreateProject(ctx, "Beta Project", "Project on the beta instance", bob.ID)
	require.NoError(t, err)

	discoveredProject, err := alpha.remoteActorService.Discover(ctx, "acct:project-"+betaProject.ID+"@"+beta.domain)
	require.NoError(t, err)
	assert.Equal(t, betaProject.APID, discoveredProject.APID)
	assert.Equal(t, "Group", discoveredProject.Type)
	assert.Equal(t, activitypub.Inbox(betaProject.APID), discoveredProject.InboxURL)
	requireRemoteActorCached(t, alpha.db, betaProject.APID)

	followID := activitypub.ActivityAPID(alpha.cfg, "follow-beta-project")
	followBody := marshalLocalFederationJSON(t, map[string]any{
		"@context": activitypub.Context(),
		"id":       followID,
		"type":     "Follow",
		"actor":    alice.APID,
		"object":   betaProject.APID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, activitypub.Inbox(betaProject.APID), bytes.NewReader(followBody))
	require.NoError(t, err)
	req.Header.Set(echo.HeaderContentType, activitypub.ActivityJSONMediaType)
	require.NoError(t, alpha.signatureService.SignRequest(ctx, alice.ID, req, followBody))

	resp, err := router.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusAccepted, resp.StatusCode, string(rawResponse))
	assert.Contains(t, resp.Header.Get(echo.HeaderContentType), "application/json")
	assert.Equal(t, followID, localFederationJSONField(t, string(rawResponse), "activity_ap_id"))
	assert.Equal(t, 1, router.requests[beta.domain+"/.well-known/webfinger"])
	assert.GreaterOrEqual(t, router.requests[beta.domain+"/projects/"+betaProject.ID], 1)
	assert.GreaterOrEqual(t, router.requests[alpha.domain+"/users/"+alice.Username], 1)
	requireRemoteActorCached(t, beta.db, alice.APID)
	requireInboxItem(t, beta.db, betaProject.ID, "Follow", betaProject.APID)
}

func TestLocalThreeInstanceProjectFanOut(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests against a migrated PostgreSQL database")
	}

	ctx := context.Background()
	router := &localFederationRouter{handlers: make(map[string]http.Handler)}
	alpha := newLocalFederationInstance(t, ctx, source, "alpha_fanout", "alpha.local.test", router)
	beta := newLocalFederationInstance(t, ctx, source, "beta_fanout", "beta.local.test", router)
	gamma := newLocalFederationInstance(t, ctx, source, "gamma_fanout", "gamma.local.test", router)
	router.handlers[alpha.domain] = alpha.echo
	router.handlers[beta.domain] = beta.echo
	router.handlers[gamma.domain] = gamma.echo

	alice, err := alpha.userService.RegisterUser(ctx, "alice-fanout", "alice-fanout@alpha.local.test", "password123")
	require.NoError(t, err)
	bob, err := beta.userService.RegisterUser(ctx, "bob-fanout", "bob-fanout@beta.local.test", "password123")
	require.NoError(t, err)
	gina, err := gamma.userService.RegisterUser(ctx, "gina-fanout", "gina-fanout@gamma.local.test", "password123")
	require.NoError(t, err)
	betaProject, err := beta.projectService.CreateProject(ctx, "Beta Fan-Out Project", "Project on the beta instance", bob.ID)
	require.NoError(t, err)

	alphaRemoteProject := sendLocalFollow(t, ctx, router, alpha, alice.ID, alice.APID, betaProject.APID)
	gammaRemoteProject := sendLocalFollow(t, ctx, router, gamma, gina.ID, gina.APID, betaProject.APID)
	requireRemoteActorCached(t, beta.db, alice.APID)
	requireRemoteActorCached(t, beta.db, gina.APID)
	acceptAndDeliverProjectFollow(t, ctx, router, beta, alice.APID, betaProject.ID)
	acceptAndDeliverProjectFollow(t, ctx, router, beta, gina.APID, betaProject.ID)
	requireFollow(t, beta.db, requireActorIDByAPID(t, beta.db, alice.APID), betaProject.ID, "accepted")
	requireFollow(t, beta.db, requireActorIDByAPID(t, beta.db, gina.APID), betaProject.ID, "accepted")
	requireFollow(t, alpha.db, alice.ID, alphaRemoteProject.ID, "accepted")
	requireFollow(t, gamma.db, gina.ID, gammaRemoteProject.ID, "accepted")

	ticketAPID := alpha.cfg.BaseURL + "/tickets/fanout-ticket-1"
	createActivityAPID := alpha.cfg.BaseURL + "/activities/fanout-create-ticket-1"
	createBody := marshalLocalFederationJSON(t, map[string]any{
		"@context": activitypub.Context(),
		"id":       createActivityAPID,
		"type":     "Create",
		"actor":    alice.APID,
		"target":   betaProject.APID,
		"object": map[string]any{
			"id":               ticketAPID,
			"type":             []string{"forge:Ticket"},
			"attributedTo":     alice.APID,
			"context":          betaProject.APID,
			"name":             "Fan-out task",
			"content":          "Created on alpha and fanned out by beta.",
			"forge:priority":   "high",
			"forge:ticketType": "task",
			"forge:isResolved": false,
		},
	})
	createResp := postLocalSignedActivity(t, ctx, router, alpha, alice.ID, activitypub.Inbox(betaProject.APID), createBody)
	defer createResp.Body.Close()
	rawCreateResp, err := io.ReadAll(createResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, createResp.StatusCode, string(rawCreateResp))

	requireObjectType(t, beta.db, ticketAPID, "Ticket")
	requireInboxItem(t, beta.db, betaProject.ID, "Create", ticketAPID)
	requireDeliveryForObjectFromActor(t, beta.db, "Create", ticketAPID, activitypub.Inbox(gina.APID), betaProject.ID)
	requireNoDeliveryForObject(t, beta.db, "Create", ticketAPID, activitypub.Inbox(alice.APID))

	deliveryID := requireDeliveryIDForTarget(t, beta.db, ticketAPID, activitypub.Inbox(gina.APID))
	worker := delivery.NewWorker(
		delivery.NewRepository(beta.db),
		beta.signatureService,
		router,
		delivery.WithRemoteActorRefresher(beta.remoteActorService),
		delivery.WithTargetActorRefreshMaxAge(0),
	)
	require.NoError(t, worker.HandleDeliveryTask(ctx, federationDeliveryTask(t, deliveryID)))
	requireInboxItem(t, gamma.db, gina.ID, "Create", ticketAPID)
	requireRemoteActorCached(t, gamma.db, betaProject.APID)
}

type localFederationInstance struct {
	name               string
	domain             string
	db                 *sqlx.DB
	cfg                activitypub.Config
	echo               *echo.Echo
	userService        *user.Service
	projectService     *project.Service
	remoteActorService *remoteactor.Service
	signatureService   *httpsig.Service
}

func sendLocalFollow(t *testing.T, ctx context.Context, router *localFederationRouter, source *localFederationInstance, actorID, actorAPID, projectAPID string) *remoteactor.Actor {
	t.Helper()

	targetActor, err := source.remoteActorService.Fetch(ctx, projectAPID)
	require.NoError(t, err)

	followActivityID, err := activitypub.NewID()
	require.NoError(t, err)
	followID := activitypub.ActivityAPID(source.cfg, followActivityID)
	body := marshalLocalFederationJSON(t, map[string]any{
		"@context": activitypub.Context(),
		"id":       followID,
		"type":     "Follow",
		"actor":    actorAPID,
		"object":   projectAPID,
	})
	storeLocalOutgoingFollow(t, ctx, source.db, actorID, targetActor.ID, followActivityID, followID, projectAPID, body)

	resp := postLocalSignedActivity(t, ctx, router, source, actorID, activitypub.Inbox(projectAPID), body)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode, string(raw))
	return targetActor
}

func postLocalSignedActivity(t *testing.T, ctx context.Context, router *localFederationRouter, source *localFederationInstance, actorID, inboxURL string, body []byte) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inboxURL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(echo.HeaderContentType, activitypub.ActivityJSONMediaType)
	require.NoError(t, source.signatureService.SignRequest(ctx, actorID, req, body))
	resp, err := router.Do(req)
	require.NoError(t, err)
	return resp
}

func storeLocalOutgoingFollow(t *testing.T, ctx context.Context, db *sqlx.DB, actorID, targetActorID, followActivityID, followAPID, projectAPID string, body []byte) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
		INSERT INTO ap_activities (id, ap_id, activity_type, actor_id, object_ap_id, document)
		VALUES ($1, $2, 'Follow', $3, $4, $5)
	`, followActivityID, followAPID, actorID, projectAPID, body)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_outbox_items (actor_id, activity_id, activity_ap_id)
		VALUES ($1, $2, $3)
	`, actorID, followActivityID, followAPID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'pending')
	`, actorID, targetActorID)
	require.NoError(t, err)
}

func acceptAndDeliverProjectFollow(t *testing.T, ctx context.Context, router *localFederationRouter, instance *localFederationInstance, followerAPID, projectID string) {
	t.Helper()

	follow := requireProjectFollowActivity(t, ctx, instance.db, projectID, followerAPID)
	inbound := &remoteinbox.InboundActivity{
		ID:         follow.FollowAPID,
		Type:       "Follow",
		ActorAPID:  follow.FollowerAPID,
		ActorID:    follow.FollowerID,
		ObjectAPID: &follow.ProjectAPID,
	}
	response, err := remoteinbox.NewRepository(instance.db, instance.cfg).AcceptProjectFollow(ctx, follow.ProjectActorID, inbound)
	require.NoError(t, err)

	deliveryService := delivery.NewService(delivery.NewRecipientRepository(instance.db), delivery.NoopQueue{})
	queued, err := deliveryService.Enqueue(ctx, response.ActivityID, response.TargetInboxURL)
	require.NoError(t, err)

	worker := delivery.NewWorker(
		delivery.NewRepository(instance.db),
		instance.signatureService,
		router,
		delivery.WithRemoteActorRefresher(instance.remoteActorService),
		delivery.WithTargetActorRefreshMaxAge(0),
	)
	require.NoError(t, worker.HandleDeliveryTask(ctx, federationDeliveryTask(t, queued.ID)))
}

type localProjectFollowActivity struct {
	ProjectActorID string `db:"project_actor_id"`
	ProjectAPID    string `db:"project_ap_id"`
	FollowerID     string `db:"follower_id"`
	FollowerAPID   string `db:"follower_ap_id"`
	FollowAPID     string `db:"follow_ap_id"`
}

func requireProjectFollowActivity(t *testing.T, ctx context.Context, db *sqlx.DB, projectID, followerAPID string) localProjectFollowActivity {
	t.Helper()

	var follow localProjectFollowActivity
	require.NoError(t, db.GetContext(ctx, &follow, `
		SELECT
			project_actor.id::text AS project_actor_id,
			project_actor.ap_id AS project_ap_id,
			follower.id::text AS follower_id,
			follower.ap_id AS follower_ap_id,
			activity.ap_id AS follow_ap_id
		FROM projects project
		JOIN actors project_actor ON project_actor.id = project.id
		JOIN actor_inbox_items inbox_item ON inbox_item.actor_id = project_actor.id
		JOIN ap_activities activity ON activity.id = inbox_item.activity_id
		JOIN actors follower ON follower.id = activity.actor_id
		WHERE project.id = $1
			AND activity.activity_type = 'Follow'
			AND activity.object_ap_id = project_actor.ap_id
			AND follower.ap_id = $2
		ORDER BY inbox_item.received_at DESC
		LIMIT 1
	`, projectID, followerAPID))
	return follow
}

func requireActorIDByAPID(t *testing.T, db *sqlx.DB, actorAPID string) string {
	t.Helper()

	var actorID string
	require.NoError(t, db.Get(&actorID, `SELECT id::text FROM actors WHERE ap_id = $1`, actorAPID))
	return actorID
}

func lastLocalFederationPath(apID string) string {
	idx := strings.LastIndex(apID, "/")
	if idx == -1 || idx == len(apID)-1 {
		return apID
	}
	return apID[idx+1:]
}

func newLocalFederationInstance(t *testing.T, ctx context.Context, source, name, domain string, router *localFederationRouter) *localFederationInstance {
	t.Helper()

	schema := "test_federation_" + name
	db := newSchemaIntegrationDB(t, ctx, source, schema)
	cfg := activitypub.NewConfig("https://"+domain, domain)

	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)
	ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
	commentService := comment.NewService(comment.NewRepository(db, cfg), ticketService, cfg)
	deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
	projectService.SetDelivery(deliveryService)
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)

	remoteActorService := remoteactor.NewService(
		remoteactor.NewRepository(db),
		remoteactor.WithHTTPClient(router),
		remoteactor.WithWebFingerScheme("https"),
	)
	signatureService := httpsig.NewService(
		httpsig.NewRepository(db),
		httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
		httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
	)

	e := echo.New()
	webfinger.NewHandler(webfinger.NewService(webfinger.NewRepository(db), cfg)).RegisterRoutes(e)
	activitypub.NewHandler(db, cfg).RegisterRoutes(e)
	c2s.NewHandler(db, cfg, ticketService, commentService).RegisterRoutes(e)
	remoteinbox.NewHandler(
		remoteinbox.NewService(remoteinbox.NewRepository(db, cfg), signatureService, remoteinbox.WithDelivery(deliveryService)),
		cfg,
	).RegisterRoutes(e)

	return &localFederationInstance{
		name:               name,
		domain:             domain,
		db:                 db,
		cfg:                cfg,
		echo:               e,
		userService:        userService,
		projectService:     projectService,
		remoteActorService: remoteActorService,
		signatureService:   signatureService,
	}
}

func newSchemaIntegrationDB(t *testing.T, ctx context.Context, source, schema string) *sqlx.DB {
	t.Helper()

	admin, err := sqlx.Connect("postgres", source)
	require.NoError(t, err)
	defer admin.Close()

	quotedSchema := pq.QuoteIdentifier(schema)
	_, err = admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
	require.NoError(t, err)
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+quotedSchema)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupDB, cleanupErr := sqlx.Connect("postgres", source)
		if cleanupErr == nil {
			_, _ = cleanupDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
			_ = cleanupDB.Close()
		}
	})

	db, err := sqlx.Connect("postgres", source)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.ExecContext(ctx, "SET search_path TO "+quotedSchema)
	require.NoError(t, err)
	runSchemaMigrations(t, ctx, db)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func runSchemaMigrations(t *testing.T, ctx context.Context, db *sqlx.DB) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)
	for _, file := range files {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(raw))
		require.NoErrorf(t, err, "migration %s failed", filepath.Base(file))
	}
}

type localFederationRouter struct {
	handlers map[string]http.Handler
	requests map[string]int
}

func (r *localFederationRouter) Do(req *http.Request) (*http.Response, error) {
	if r.requests == nil {
		r.requests = make(map[string]int)
	}
	if req.URL == nil {
		return nil, fmt.Errorf("request url is required")
	}
	handler := r.handlers[req.URL.Host]
	if handler == nil {
		return localFederationResponse(http.StatusNotFound, `{"error":"unknown local federation host"}`), nil
	}
	r.requests[req.URL.Host+req.URL.Path]++

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, cloneLocalFederationRequest(req))
	return rec.Result(), nil
}

func cloneLocalFederationRequest(req *http.Request) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.URL = &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawPath:  req.URL.RawPath,
		RawQuery: req.URL.RawQuery,
		Fragment: req.URL.Fragment,
	}
	cloned.RequestURI = ""
	cloned.Host = req.URL.Host
	return cloned
}

func localFederationResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func marshalLocalFederationJSON(t *testing.T, value any) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func localFederationJSONField(t *testing.T, raw, key string) string {
	t.Helper()

	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &response))
	value, ok := response[key].(string)
	require.True(t, ok, "missing string field %s in %s", key, raw)
	return value
}

func requireRemoteActorCached(t *testing.T, db *sqlx.DB, actorAPID string) {
	t.Helper()

	var exists bool
	require.NoError(t, db.Get(&exists, `
		SELECT EXISTS(
			SELECT 1
			FROM actors
			WHERE ap_id = $1 AND is_local = false
		)
	`, actorAPID))
	require.True(t, exists, "remote actor %s should be cached", actorAPID)
}

//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/httpsig"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteactor"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/remoteinbox"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityPubFoundationFlow(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userRepo := user.NewRepository(db, cfg)
	userService := user.NewService(userRepo, []byte("integration-secret"), cfg)

	projectRepo := project.NewRepository(db, cfg)
	projectService := project.NewService(projectRepo, cfg)

	ticketRepo := ticket.NewRepository(db, cfg)
	ticketService := ticket.NewService(ticketRepo, projectService, cfg)

	commentRepo := comment.NewRepository(db, cfg)
	commentService := comment.NewService(commentRepo, ticketService, cfg)

	deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
	ticketService.SetDelivery(deliveryService)
	commentService.SetDelivery(deliveryService)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	member, err := userService.RegisterUser(ctx, "member", "member@example.test", "password123")
	require.NoError(t, err)

	requireRowCount(t, db, "actors", 2)
	requireRowCount(t, db, "actor_keys", 2)
	requireObjectType(t, db, owner.APID, "Person")

	project, err := projectService.CreateProject(ctx, "Federated Board", "A local ActivityPub board", owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, project.APID, "Group")
	requireProjectRole(t, db, owner.ID, project.ID, "owner")
	requireFollow(t, db, owner.ID, project.ID, "accepted")

	invite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, member.ID, "developer")
	require.NoError(t, err)
	requireActivityType(t, db, invite.APID, "Invite")

	require.NoError(t, projectService.AcceptInvite(ctx, invite.ID, member.ID))
	requireProjectRole(t, db, member.ID, project.ID, "developer")
	requireFollow(t, db, member.ID, project.ID, "accepted")

	remoteInbox := "https://remote.example/users/follower/inbox"
	remoteFollower := &remoteactor.Actor{
		APID:              "https://remote.example/users/follower",
		Type:              "Person",
		PreferredUsername: "follower",
		Handle:            "follower@remote.example",
		Name:              "Remote Follower",
		InboxURL:          remoteInbox,
		OutboxURL:         "https://remote.example/users/follower/outbox",
		PublicKeyID:       "https://remote.example/users/follower#main-key",
		PublicKeyPEM:      "public key",
		Document:          []byte(`{"id":"https://remote.example/users/follower","type":"Person"}`),
	}
	require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteFollower))
	_, err = db.ExecContext(ctx, `
		INSERT INTO actor_follows (follower_actor_id, followed_actor_id, state)
		VALUES ($1, $2, 'accepted')
	`, remoteFollower.ID, project.ID)
	require.NoError(t, err)

	createdTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
		Title:       "Design local outbox",
		Description: "Persist local AP activities",
		Priority:    "high",
		Type:        "task",
	}, project.ID, owner.ID)
	require.NoError(t, err)
	requireObjectType(t, db, createdTicket.APID, "Ticket")
	requireActivityForObject(t, db, "Create", createdTicket.APID)
	requireInboxItem(t, db, project.ID, "Create", createdTicket.APID)
	requireOutboxItem(t, db, owner.ID, "Create", createdTicket.APID)
	requireDeliveryForObject(t, db, "Create", createdTicket.APID, remoteInbox)

	status := "done"
	assigneeID := member.ID
	assigneePatch := &assigneeID
	require.NoError(t, ticketService.UpdateTicket(ctx, ticket.UpdateTicketRequest{
		Status:     &status,
		AssigneeID: &assigneePatch,
	}, createdTicket.ID, owner.ID))
	requireActivityForObject(t, db, "Update", createdTicket.APID)
	requireDeliveryForObject(t, db, "Update", createdTicket.APID, remoteInbox)
	requireTicketResolved(t, db, createdTicket.ID)

	createdComment, err := commentService.CreateComment(ctx, createdTicket.ID, member.ID, "This is ready.")
	require.NoError(t, err)
	requireObjectType(t, db, createdComment.APID, "Note")
	requireActivityForObject(t, db, "Create", createdComment.APID)
	requireOutboxItem(t, db, member.ID, "Create", createdComment.APID)
	requireDeliveryForObject(t, db, "Create", createdComment.APID, remoteInbox)
}

func TestActivityPubFoundationConstraints(t *testing.T) {
	db := openIntegrationDB(t)
	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")

	userRepo := user.NewRepository(db, cfg)
	userService := user.NewService(userRepo, []byte("integration-secret"), cfg)
	projectRepo := project.NewRepository(db, cfg)
	projectService := project.NewService(projectRepo, cfg)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	project, err := projectService.CreateProject(ctx, "Constraint Board", "", owner.ID)
	require.NoError(t, err)

	t.Run("core tables exist", func(t *testing.T) {
		for _, tableName := range []string{
			"actors",
			"actor_keys",
			"ap_objects",
			"ap_activities",
			"activity_deliveries",
			"actor_inbox_items",
			"actor_outbox_items",
			"project_members",
			"actor_follows",
			"project_invites",
			"tickets",
			"ticket_assignees",
			"comments",
		} {
			var exists bool
			err := db.Get(&exists, `SELECT to_regclass($1) IS NOT NULL`, tableName)
			require.NoError(t, err)
			assert.True(t, exists, tableName)
		}
	})

	t.Run("duplicate actor handle fails", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO actors (
				ap_id, type, preferred_username, handle, name, inbox_url, outbox_url, followers_url
			)
			VALUES (
				$1, 'Person', 'owner-copy', $2, 'Owner Copy', $3, $4, $5
			)
		`,
			"http://localhost:8080/users/owner-copy",
			owner.Handle,
			"http://localhost:8080/users/owner-copy/inbox",
			"http://localhost:8080/users/owner-copy/outbox",
			"http://localhost:8080/users/owner-copy/followers",
		)
		require.Error(t, err)
	})

	t.Run("invalid ticket status fails", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO tickets (
				ap_id, project_id, reporter_id, title, status, priority, type
			)
			VALUES ($1, $2, $3, 'Invalid status', 'blocked', 'medium', 'task')
		`, "http://localhost:8080/tickets/invalid-status", project.ID, owner.ID)
		require.Error(t, err)
	})

	t.Run("remote service actor allows public only key", func(t *testing.T) {
		repo := remoteactor.NewRepository(db)
		followersURL := "https://remote.example/users/bot/followers"
		actor := &remoteactor.Actor{
			APID:              "https://remote.example/users/bot",
			Type:              "Service",
			PreferredUsername: "bot",
			Handle:            "bot@remote.example",
			Name:              "Remote Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/bot/inbox",
			OutboxURL:         "https://remote.example/users/bot/outbox",
			FollowersURL:      &followersURL,
			PublicKeyID:       "https://remote.example/users/bot#main-key",
			PublicKeyPEM:      "public key",
			Document:          []byte(`{"id":"https://remote.example/users/bot","type":"Service"}`),
		}

		require.NoError(t, repo.UpsertRemoteActor(ctx, actor))

		var isLocal bool
		require.NoError(t, db.GetContext(ctx, &isLocal, `
			SELECT is_local FROM actors WHERE ap_id = $1
		`, actor.APID))
		assert.False(t, isLocal)

		var privateKey *string
		require.NoError(t, db.GetContext(ctx, &privateKey, `
			SELECT private_key_pem FROM actor_keys WHERE key_id = $1
		`, actor.PublicKeyID))
		assert.Nil(t, privateKey)
	})

	t.Run("remote actors can author tickets and comments", func(t *testing.T) {
		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/author",
			Type:              "Person",
			PreferredUsername: "author",
			Handle:            "author@remote.example",
			Name:              "Remote Author",
			Summary:           "",
			InboxURL:          "https://remote.example/users/author/inbox",
			OutboxURL:         "https://remote.example/users/author/outbox",
			PublicKeyID:       "https://remote.example/users/author#main-key",
			PublicKeyPEM:      "public key",
			Document:          []byte(`{"id":"https://remote.example/users/author","type":"Person"}`),
		}
		require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteActor))

		ticketID, err := activitypub.NewID()
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO tickets (
				id, ap_id, project_id, reporter_id, title, status, priority, type
			)
			VALUES ($1, $2, $3, $4, 'Remote authored ticket', 'open', 'medium', 'task')
		`, ticketID, activitypub.TicketAPID(cfg, ticketID), project.ID, remoteActor.ID)
		require.NoError(t, err)

		commentID, err := activitypub.NewID()
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO comments (id, ap_id, ticket_id, author_id, content)
			VALUES ($1, $2, $3, $4, 'Remote authored comment')
		`, commentID, activitypub.CommentAPID(cfg, commentID), ticketID, remoteActor.ID)
		require.NoError(t, err)
	})

	t.Run("remote inbox stores inbound activity idempotently", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		remoteActorRepo := remoteactor.NewRepository(db)
		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/inbox-bot",
			Type:              "Service",
			PreferredUsername: "inbox-bot",
			Handle:            "inbox-bot@remote.example",
			Name:              "Inbox Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/inbox-bot/inbox",
			OutboxURL:         "https://remote.example/users/inbox-bot/outbox",
			PublicKeyID:       "https://remote.example/users/inbox-bot#main-key",
			PublicKeyPEM:      publicKey,
			Document:          []byte(`{"id":"https://remote.example/users/inbox-bot","type":"Service"}`),
		}
		require.NoError(t, remoteActorRepo.UpsertRemoteActor(ctx, remoteActor))

		inboxRepo := remoteinbox.NewRepository(db)
		objectAPID := "https://remote.example/notes/1"
		body := []byte(`{"id":"https://remote.example/activities/inbox-create","type":"Create","actor":"https://remote.example/users/inbox-bot","object":"https://remote.example/notes/1"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, req, body))

		receiver := remoteinbox.NewService(inboxRepo, httpsig.NewService(httpsig.NewRepository(db)))
		first, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		assert.False(t, first.Duplicate)

		second, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		assert.True(t, second.Duplicate)

		requireInboxItem(t, db, project.ID, "Create", objectAPID)
	})

	t.Run("remote inbox accepts project follow and queues response", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		remoteActor := &remoteactor.Actor{
			APID:              "https://remote.example/users/follow-bot",
			Type:              "Person",
			PreferredUsername: "follow-bot",
			Handle:            "follow-bot@remote.example",
			Name:              "Follow Bot",
			Summary:           "",
			InboxURL:          "https://remote.example/users/follow-bot/inbox",
			OutboxURL:         "https://remote.example/users/follow-bot/outbox",
			PublicKeyID:       "https://remote.example/users/follow-bot#main-key",
			PublicKeyPEM:      publicKey,
			Document:          []byte(`{"id":"https://remote.example/users/follow-bot","type":"Person"}`),
		}
		require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, remoteActor))

		followAPID := "https://remote.example/activities/follow-project"
		body := []byte(`{"id":"` + followAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + project.APID + `"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, req, body))

		deliveryService := delivery.NewService(delivery.NewRecipientRepository(db), delivery.NoopQueue{})
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(httpsig.NewRepository(db)),
			remoteinbox.WithDelivery(deliveryService),
		)
		accepted, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		require.NotEmpty(t, accepted.ResponseActivityID)

		requireFollow(t, db, remoteActor.ID, project.ID, "accepted")
		requireActivityForObject(t, db, "Accept", followAPID)
		requireOutboxItem(t, db, project.ID, "Accept", followAPID)
		requireDeliveryForObject(t, db, "Accept", followAPID, remoteActor.InboxURL)

		ticketService := ticket.NewService(ticket.NewRepository(db, cfg), projectService, cfg)
		ticketService.SetDelivery(deliveryService)
		localTicket, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
			Title:       "Remote comment target",
			Description: "A local ticket for remote discussion",
			Priority:    "medium",
			Type:        "task",
		}, project.ID, owner.ID)
		require.NoError(t, err)

		noteAPID := "https://remote.example/notes/project-comment"
		createNoteAPID := "https://remote.example/activities/create-project-comment"
		noteBody := []byte(`{"id":"` + createNoteAPID + `","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"` + noteAPID + `","type":"Note","attributedTo":"` + remoteActor.APID + `","inReplyTo":"` + localTicket.APID + `","content":"Remote review looks good."}}`)
		noteReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(noteBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, noteReq, noteBody))

		remoteComment, err := receiver.Receive(ctx, noteReq, project.APID, noteBody)
		require.NoError(t, err)
		require.Equal(t, createNoteAPID, remoteComment.ActivityAPID)
		requireObjectType(t, db, noteAPID, "Note")
		requireActivityForObject(t, db, "Create", noteAPID)
		requireInboxItem(t, db, project.ID, "Create", noteAPID)

		commentService := comment.NewService(comment.NewRepository(db, cfg), ticketService, cfg)
		commentService.SetDelivery(deliveryService)
		comments, err := commentService.ListComments(ctx, localTicket.ID, owner.ID)
		require.NoError(t, err)
		require.Len(t, comments, 1)
		assert.Equal(t, noteAPID, comments[0].APID)
		assert.Equal(t, remoteActor.ID, comments[0].AuthorID)
		assert.Equal(t, "Remote review looks good.", comments[0].Content)

		remoteTicketAPID := "https://remote.example/tickets/project-ticket"
		createTicketAPID := "https://remote.example/activities/create-project-ticket"
		ticketBody := []byte(`{"id":"` + createTicketAPID + `","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"` + remoteTicketAPID + `","type":["forge:Ticket"],"attributedTo":"` + remoteActor.APID + `","context":"` + project.APID + `","name":"Remote ticket","content":"Created from another server.","forge:priority":"urgent","forge:ticketType":"task","forge:isResolved":false}}`)
		ticketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(ticketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, ticketReq, ticketBody))

		remoteTicket, err := receiver.Receive(ctx, ticketReq, project.APID, ticketBody)
		require.NoError(t, err)
		require.Equal(t, createTicketAPID, remoteTicket.ActivityAPID)
		requireObjectType(t, db, remoteTicketAPID, "Ticket")
		requireActivityForObject(t, db, "Create", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Create", remoteTicketAPID)

		tickets, err := ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		createdRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, createdRemoteTicket)
		assert.Equal(t, remoteActor.ID, createdRemoteTicket.ReporterID)
		assert.Equal(t, "Remote ticket", createdRemoteTicket.Title)
		assert.Equal(t, "Created from another server.", createdRemoteTicket.Description)
		assert.Equal(t, "urgent", createdRemoteTicket.Priority)
		assert.Equal(t, "task", createdRemoteTicket.Type)

		undoAPID := "https://remote.example/activities/undo-follow-project"
		undoBody := []byte(`{"id":"` + undoAPID + `","type":"Undo","actor":"` + remoteActor.APID + `","object":{"id":"` + followAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + project.APID + `"}}`)
		undoReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(undoBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, undoReq, undoBody))

		undo, err := receiver.Receive(ctx, undoReq, project.APID, undoBody)
		require.NoError(t, err)
		require.Empty(t, undo.ResponseActivityID)

		requireNoFollow(t, db, remoteActor.ID, project.ID)
		requireActivityForObject(t, db, "Undo", followAPID)
		requireInboxItem(t, db, project.ID, "Undo", followAPID)

		localReply, err := commentService.CreateComment(ctx, localTicket.ID, owner.ID, "Thanks for the review.")
		require.NoError(t, err)
		requireDeliveryForObject(t, db, "Create", localReply.APID, remoteActor.InboxURL)

		ticketAfterUndo, err := ticketService.CreateTicket(ctx, ticket.CreateTicketRequest{
			Title:       "No delivery after undo",
			Description: "Remote follower opted out",
			Priority:    "medium",
			Type:        "task",
		}, project.ID, owner.ID)
		require.NoError(t, err)
		requireNoDeliveryForObject(t, db, "Create", ticketAfterUndo.APID, remoteActor.InboxURL)
	})

	t.Run("outbox delivery ledger tracks attempts", func(t *testing.T) {
		var activityID string
		require.NoError(t, db.GetContext(ctx, &activityID, `
			SELECT id::text
			FROM ap_activities
			WHERE activity_type = 'Create' AND object_ap_id = $1
			LIMIT 1
		`, project.APID))

		repo := delivery.NewRepository(db)
		created, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/bot/inbox", 3)
		require.NoError(t, err)
		assert.True(t, isNew)
		assert.Equal(t, delivery.StatePending, created.State)

		duplicate, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/bot/inbox", 3)
		require.NoError(t, err)
		assert.False(t, isNew)
		assert.Equal(t, created.ID, duplicate.ID)

		started, err := repo.StartAttempt(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, delivery.StateProcessing, started.State)
		assert.Equal(t, 1, started.Attempts)

		nextAttempt := time.Now().UTC().Add(time.Minute)
		require.NoError(t, repo.MarkFailed(ctx, created.ID, "remote 503", &nextAttempt))

		retried, err := repo.StartAttempt(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, retried.Attempts)

		require.NoError(t, repo.MarkDelivered(ctx, created.ID))
		_, err = repo.StartAttempt(ctx, created.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryDone)
	})
}

type singleKeyRepo struct {
	key *httpsig.ActorKey
}

func (r singleKeyRepo) ActivePrivateKey(ctx context.Context, actorID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func (r singleKeyRepo) PublicKeyByKeyID(ctx context.Context, keyID string) (*httpsig.ActorKey, error) {
	return r.key, nil
}

func openIntegrationDB(t *testing.T) *sqlx.DB {
	t.Helper()

	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to run integration tests against a migrated PostgreSQL database")
	}

	db, err := sqlx.Connect("postgres", source)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func resetIntegrationDB(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		TRUNCATE TABLE
			actor_outbox_items,
			actor_inbox_items,
			project_invites,
			activity_deliveries,
			ap_activities,
			ap_objects,
			comments,
			ticket_assignees,
			ticket_links,
			tickets,
			actor_follows,
			project_members,
			projects,
			users,
			actor_keys,
			actors
		CASCADE
	`)
	require.NoError(t, err)
}

func requireRowCount(t *testing.T, db *sqlx.DB, tableName string, expected int) {
	t.Helper()

	var actual int
	err := db.Get(&actual, "SELECT count(*) FROM "+tableName)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func findTicketByAPID(tickets []ticket.Ticket, apID string) *ticket.Ticket {
	for i := range tickets {
		if tickets[i].APID == apID {
			return &tickets[i]
		}
	}
	return nil
}

func requireObjectType(t *testing.T, db *sqlx.DB, apID, expectedType string) {
	t.Helper()

	var objectType string
	err := db.Get(&objectType, `SELECT object_type FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedType, objectType)
}

func requireProjectRole(t *testing.T, db *sqlx.DB, userID, projectID, expectedRole string) {
	t.Helper()

	var role string
	err := db.Get(&role, `
		SELECT role
		FROM project_members
		WHERE user_id = $1 AND project_id = $2
	`, userID, projectID)
	require.NoError(t, err)
	require.Equal(t, expectedRole, role)
}

func requireFollow(t *testing.T, db *sqlx.DB, followerID, followedID, expectedState string) {
	t.Helper()

	var state string
	err := db.Get(&state, `
		SELECT state
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, followerID, followedID)
	require.NoError(t, err)
	require.Equal(t, expectedState, state)
}

func requireNoFollow(t *testing.T, db *sqlx.DB, followerID, followedID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM actor_follows
		WHERE follower_actor_id = $1 AND followed_actor_id = $2
	`, followerID, followedID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireActivityType(t *testing.T, db *sqlx.DB, apID, expectedType string) {
	t.Helper()

	var activityType string
	err := db.Get(&activityType, `SELECT activity_type FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedType, activityType)
}

func requireActivityForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ap_activities
		WHERE activity_type = $1 AND object_ap_id = $2
	`, activityType, objectAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireDeliveryForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
	`, activityType, objectAPID, inboxURL)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireNoDeliveryForObject(t *testing.T, db *sqlx.DB, activityType, objectAPID, inboxURL string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM activity_deliveries delivery
		JOIN ap_activities activity ON activity.id = delivery.activity_id
		WHERE activity.activity_type = $1
			AND activity.object_ap_id = $2
			AND delivery.target_inbox_url = $3
	`, activityType, objectAPID, inboxURL)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireInboxItem(t *testing.T, db *sqlx.DB, actorID, activityType, objectAPID string) {
	t.Helper()
	requireBoxItem(t, db, "actor_inbox_items", actorID, activityType, objectAPID)
}

func requireOutboxItem(t *testing.T, db *sqlx.DB, actorID, activityType, objectAPID string) {
	t.Helper()
	requireBoxItem(t, db, "actor_outbox_items", actorID, activityType, objectAPID)
}

func requireBoxItem(t *testing.T, db *sqlx.DB, tableName, actorID, activityType, objectAPID string) {
	t.Helper()

	if tableName != "actor_inbox_items" && tableName != "actor_outbox_items" {
		t.Fatalf("unexpected box table %q", tableName)
	}

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM `+tableName+` box
		JOIN ap_activities activity ON activity.id = box.activity_id
		WHERE box.actor_id = $1
			AND activity.activity_type = $2
			AND activity.object_ap_id = $3
	`, actorID, activityType, objectAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireTicketResolved(t *testing.T, db *sqlx.DB, ticketID string) {
	t.Helper()

	var resolved sql.NullBool
	err := db.Get(&resolved, `SELECT is_resolved FROM tickets WHERE id = $1`, ticketID)
	require.NoError(t, err)
	require.True(t, resolved.Valid)
	require.True(t, resolved.Bool)
}

func TestIntegrationDBSourceLooksSafe(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to check integration database safety")
	}
	require.True(t, strings.Contains(source, "localhost") || strings.Contains(source, "127.0.0.1") || strings.Contains(source, "db:5432"))
}

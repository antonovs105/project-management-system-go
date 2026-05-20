//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
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

	ownerDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, owner.ID)
	require.NoError(t, err)
	requireProjectDelivery(t, ownerDeliveries, "Create", createdTicket.APID, remoteInbox)
	requireProjectDelivery(t, ownerDeliveries, "Update", createdTicket.APID, remoteInbox)
	requireProjectDelivery(t, ownerDeliveries, "Create", createdComment.APID, remoteInbox)

	memberDeliveries, err := deliveryService.ListProjectDeliveries(ctx, project.ID, member.ID)
	require.NoError(t, err)
	requireProjectDelivery(t, memberDeliveries, "Create", createdTicket.APID, remoteInbox)

	outsider, err := userService.RegisterUser(ctx, "outsider", "outsider@example.test", "password123")
	require.NoError(t, err)
	_, err = deliveryService.ListProjectDeliveries(ctx, project.ID, outsider.ID)
	require.ErrorIs(t, err, delivery.ErrProjectAccessDenied)
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

	t.Run("remote inbox resolves unknown actor key", func(t *testing.T) {
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/unknown-key" {
				http.NotFound(w, r)
				return
			}

			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/unknown-key",
				"unknown-key",
				"Unknown Key",
				"Resolved during signature verification.",
				publicKey,
			)
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/unknown-key"
		followAPID := server.URL + "/activities/follow-unknown-key-project"
		body := []byte(`{"id":"` + followAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + project.APID + `"}`)
		req, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		keyID := activitypub.KeyID(actorAPID)
		signer := httpsig.NewService(singleKeyRepo{key: &httpsig.ActorKey{
			ActorID:       "remote-unknown-key-before-cache",
			ActorAPID:     actorAPID,
			KeyID:         keyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}})
		require.NoError(t, signer.SignRequest(ctx, "remote-unknown-key-before-cache", req, body))

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			),
		)

		accepted, err := receiver.Receive(ctx, req, project.APID, body)
		require.NoError(t, err)
		require.Equal(t, followAPID, accepted.ActivityAPID)
		require.NotEmpty(t, accepted.ResponseActivityID)

		var storedActorID string
		require.NoError(t, db.GetContext(ctx, &storedActorID, `
			SELECT id::text
			FROM actors
			WHERE ap_id = $1 AND is_local = false
		`, actorAPID))
		requireFollow(t, db, storedActorID, project.ID, "accepted")
		requireActivityForObject(t, db, "Accept", followAPID)
		requireOutboxItem(t, db, project.ID, "Accept", followAPID)
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

		updateTicketAPID := "https://remote.example/activities/update-project-ticket"
		updateTicketBody := []byte(`{"id":"` + updateTicketAPID + `","type":"Update","actor":"` + remoteActor.APID + `","target":"` + project.APID + `","object":{"id":"` + remoteTicketAPID + `","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + project.APID + `","name":"Updated remote ticket","content":"Resolved from another server.","forge:status":"done","forge:priority":"high","forge:ticketType":"task","forge:isResolved":true}}`)
		updateTicketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(updateTicketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, updateTicketReq, updateTicketBody))

		updatedRemoteTicketActivity, err := receiver.Receive(ctx, updateTicketReq, project.APID, updateTicketBody)
		require.NoError(t, err)
		require.Equal(t, updateTicketAPID, updatedRemoteTicketActivity.ActivityAPID)
		requireObjectType(t, db, remoteTicketAPID, "Ticket")
		requireActivityForObject(t, db, "Update", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Update", remoteTicketAPID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		updatedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, updatedRemoteTicket)
		assert.Equal(t, remoteActor.ID, updatedRemoteTicket.ReporterID)
		assert.Equal(t, "Updated remote ticket", updatedRemoteTicket.Title)
		assert.Equal(t, "Resolved from another server.", updatedRemoteTicket.Description)
		assert.Equal(t, "done", updatedRemoteTicket.Status)
		assert.Equal(t, "high", updatedRemoteTicket.Priority)
		assert.True(t, updatedRemoteTicket.IsResolved)
		requireTicketResolved(t, db, updatedRemoteTicket.ID)

		assignee, err := userService.RegisterUser(ctx, "assignee", "assignee@example.test", "password123")
		require.NoError(t, err)
		assigneeInvite, err := projectService.AddMemberToProject(ctx, project.ID, owner.ID, assignee.ID, "developer")
		require.NoError(t, err)
		require.NoError(t, projectService.AcceptInvite(ctx, assigneeInvite.ID, assignee.ID))

		addAssigneeAPID := "https://remote.example/activities/add-project-ticket-assignee"
		addAssigneeBody := []byte(`{"id":"` + addAssigneeAPID + `","type":"Add","actor":"` + remoteActor.APID + `","object":"` + assignee.APID + `","target":"` + remoteTicketAPID + `"}`)
		addAssigneeReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(addAssigneeBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, addAssigneeReq, addAssigneeBody))

		addedAssignee, err := receiver.Receive(ctx, addAssigneeReq, project.APID, addAssigneeBody)
		require.NoError(t, err)
		require.Equal(t, addAssigneeAPID, addedAssignee.ActivityAPID)
		requireActivityForObjectAndTarget(t, db, "Add", assignee.APID, remoteTicketAPID)
		requireInboxItemForTarget(t, db, project.ID, "Add", remoteTicketAPID)
		requireTicketAssignee(t, db, remoteTicketAPID, assignee.ID)
		requireTicketObjectAssignedTo(t, db, remoteTicketAPID, assignee.APID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assignedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, assignedRemoteTicket)
		require.NotNil(t, assignedRemoteTicket.AssigneeID)
		assert.Equal(t, assignee.ID, *assignedRemoteTicket.AssigneeID)

		removeAssigneeAPID := "https://remote.example/activities/remove-project-ticket-assignee"
		removeAssigneeBody := []byte(`{"id":"` + removeAssigneeAPID + `","type":"Remove","actor":"` + remoteActor.APID + `","object":"` + assignee.APID + `","target":"` + remoteTicketAPID + `"}`)
		removeAssigneeReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(removeAssigneeBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, removeAssigneeReq, removeAssigneeBody))

		removedAssignee, err := receiver.Receive(ctx, removeAssigneeReq, project.APID, removeAssigneeBody)
		require.NoError(t, err)
		require.Equal(t, removeAssigneeAPID, removedAssignee.ActivityAPID)
		requireActivityForObjectAndTarget(t, db, "Remove", assignee.APID, remoteTicketAPID)
		requireInboxItemForTarget(t, db, project.ID, "Remove", remoteTicketAPID)
		requireNoTicketAssignee(t, db, remoteTicketAPID, assignee.ID)
		requireTicketObjectNotAssignedTo(t, db, remoteTicketAPID, assignee.APID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		unassignedRemoteTicket := findTicketByAPID(tickets, remoteTicketAPID)
		require.NotNil(t, unassignedRemoteTicket)
		assert.Nil(t, unassignedRemoteTicket.AssigneeID)

		deleteTicketAPID := "https://remote.example/activities/delete-project-ticket"
		deleteTicketBody := []byte(`{"id":"` + deleteTicketAPID + `","type":"Delete","actor":"` + remoteActor.APID + `","object":"` + remoteTicketAPID + `","target":"` + project.APID + `"}`)
		deleteTicketReq, err := http.NewRequest(http.MethodPost, project.APID+"/inbox", bytes.NewReader(deleteTicketBody))
		require.NoError(t, err)
		require.NoError(t, signer.SignRequest(ctx, remoteActor.ID, deleteTicketReq, deleteTicketBody))

		deletedTicket, err := receiver.Receive(ctx, deleteTicketReq, project.APID, deleteTicketBody)
		require.NoError(t, err)
		require.Equal(t, deleteTicketAPID, deletedTicket.ActivityAPID)
		requireActivityForObject(t, db, "Delete", remoteTicketAPID)
		requireInboxItem(t, db, project.ID, "Delete", remoteTicketAPID)
		requireObjectDeleted(t, db, remoteTicketAPID)
		requireNoTicketByAPID(t, db, remoteTicketAPID)

		tickets, err = ticketService.ListTicketsInProject(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assert.Nil(t, findTicketByAPID(tickets, remoteTicketAPID))

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

		finalDelivery, isNew, err := repo.Create(ctx, activityID, "https://remote.example/users/dead/inbox", 2)
		require.NoError(t, err)
		assert.True(t, isNew)
		require.NoError(t, repo.MarkFailed(ctx, finalDelivery.ID, "permanent failure", nil))

		var terminal struct {
			State         string       `db:"state"`
			LastError     string       `db:"last_error"`
			NextAttemptAt sql.NullTime `db:"next_attempt_at"`
		}
		require.NoError(t, db.GetContext(ctx, &terminal, `
			SELECT state, last_error, next_attempt_at
			FROM activity_deliveries
			WHERE id = $1
		`, finalDelivery.ID))
		assert.Equal(t, delivery.StateDead, terminal.State)
		assert.Equal(t, "permanent failure", terminal.LastError)
		assert.False(t, terminal.NextAttemptAt.Valid)

		_, err = repo.StartAttempt(ctx, finalDelivery.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryExhausted)

		viewer, err := userService.RegisterUser(ctx, "retry-viewer", "retry-viewer@example.test", "password123")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `
			INSERT INTO project_members (user_id, project_id, role)
			VALUES ($1, $2, 'viewer')
		`, viewer.ID, project.ID)
		require.NoError(t, err)

		retryQueue := &recordingDeliveryQueue{}
		retryService := delivery.NewService(delivery.NewRecipientRepository(db), retryQueue)
		summary, err := retryService.GetProjectDeliverySummary(ctx, project.ID, owner.ID)
		require.NoError(t, err)
		assert.True(t, summary.CanRetry)
		assert.GreaterOrEqual(t, summary.Dead, 1)
		assert.GreaterOrEqual(t, summary.Retryable, 1)

		deadDeliveries, err := retryService.ListProjectDeliveriesWithOptions(ctx, project.ID, owner.ID, delivery.ProjectDeliveryListOptions{
			State: delivery.StateDead,
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, deadDeliveries, 1)
		assert.Equal(t, finalDelivery.ID, deadDeliveries[0].ID)
		assert.True(t, deadDeliveries[0].CanRetry)

		viewerDeadDeliveries, err := retryService.ListProjectDeliveriesWithOptions(ctx, project.ID, viewer.ID, delivery.ProjectDeliveryListOptions{
			State: delivery.StateDead,
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, viewerDeadDeliveries, 1)
		assert.False(t, viewerDeadDeliveries[0].CanRetry)

		requeued, err := retryService.RetryProjectDelivery(ctx, project.ID, owner.ID, finalDelivery.ID)
		require.NoError(t, err)
		assert.Equal(t, delivery.StatePending, requeued.State)
		assert.Zero(t, requeued.Attempts)
		assert.Equal(t, delivery.DefaultMaxRetry, requeued.MaxAttempts)
		assert.Nil(t, requeued.NextAttemptAt)
		assert.Nil(t, requeued.LastError)
		assert.Equal(t, finalDelivery.ID, retryQueue.deliveryID)
		assert.Equal(t, delivery.DefaultMaxRetry, retryQueue.maxAttempts)

		_, err = retryService.RetryProjectDelivery(ctx, project.ID, viewer.ID, finalDelivery.ID)
		require.ErrorIs(t, err, delivery.ErrDeliveryRetryDenied)
	})
}

func TestRemoteInboxRefreshesRotatedActorKey(t *testing.T) {
	db := openIntegrationDB(t)
	fx := newInboxIntegrationFixture(t, db)

	oldPublicKey, _, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)
	newPublicKey, newPrivateKey, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/rotating-key" {
			http.NotFound(w, r)
			return
		}
		doc := activitypub.ActorDocument(
			"Person",
			server.URL+"/users/rotating-key",
			"rotating-key",
			"Rotating Key",
			"Served after a key rotation.",
			newPublicKey,
		)
		rawDoc, err := activitypub.MarshalDocument(doc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/activity+json")
		_, _ = w.Write(rawDoc)
	}))
	defer server.Close()

	actorAPID := server.URL + "/users/rotating-key"
	remoteActor := createRemoteActorWithPublicKey(t, fx.Ctx, db, actorAPID, "rotating-key", oldPublicKey)
	requireActorPublicKey(t, db, remoteActor.PublicKeyID, oldPublicKey)

	activityAPID := server.URL + "/activities/follow-with-rotated-key"
	body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
	req := newInboxPostRequest(t, fx.Project.APID, body)
	signRequestWithKey(t, fx.Ctx, req, remoteActor.ID, &httpsig.ActorKey{
		ActorID:       remoteActor.ID,
		ActorAPID:     remoteActor.APID,
		KeyID:         remoteActor.PublicKeyID,
		Algorithm:     httpsig.AlgorithmRSAV15SHA256,
		PublicKeyPEM:  newPublicKey,
		PrivateKeyPEM: newPrivateKey,
	}, body)

	remoteActorService := remoteactor.NewService(
		remoteactor.NewRepository(db),
		remoteactor.WithHTTPClient(server.Client()),
	)
	receiver := remoteinbox.NewService(
		remoteinbox.NewRepository(db, fx.Cfg),
		httpsig.NewService(
			httpsig.NewRepository(db),
			httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
		),
	)

	accepted, err := receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

	require.NoError(t, err)
	require.Equal(t, activityAPID, accepted.ActivityAPID)
	require.NotEmpty(t, accepted.ResponseActivityID)
	requireFollow(t, db, remoteActor.ID, fx.Project.ID, "accepted")
	requireActivityForObject(t, db, "Accept", activityAPID)
	requireActorPublicKey(t, db, remoteActor.PublicKeyID, newPublicKey)
}

func TestRemoteInboxRejectsUnsafeInboundActivities(t *testing.T) {
	db := openIntegrationDB(t)

	t.Run("rejects signature actor mismatch without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "mismatch-signer")

		activityAPID := "https://remote.example/activities/mismatch-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"https://remote.example/users/mallory","object":"` + fx.Project.APID + `"}`)
		req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, body)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrForbiddenActor)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects missing body digest without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "missing-digest")

		activityAPID := "https://remote.example/activities/missing-digest-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRemoteInboxRequest(t, fx.Ctx, req, remoteActor, privateKey, nil)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects stale signed date without persisting", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "stale-date")

		activityAPID := "https://remote.example/activities/stale-date-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		req.Header.Set("Date", time.Now().UTC().Add(-10*time.Minute).Format(http.TimeFormat))
		signRemoteInboxRequest(t, fx.Ctx, req, remoteActor, privateKey, body)

		_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects resolved actor key mismatch without caching actor", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/key-mismatch" {
				http.NotFound(w, r)
				return
			}
			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/key-mismatch",
				"key-mismatch",
				"Key Mismatch",
				"Served with a different key id.",
				publicKey,
			)
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/key-mismatch"
		keyID := actorAPID + "#rotated-key"
		activityAPID := server.URL + "/activities/key-mismatch-follow"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRequestWithKey(t, fx.Ctx, req, "remote-key-mismatch-before-cache", &httpsig.ActorKey{
			ActorID:       "remote-key-mismatch-before-cache",
			ActorAPID:     actorAPID,
			KeyID:         keyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  publicKey,
			PrivateKeyPEM: privateKey,
		}, body)

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, fx.Cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithMissingKeyResolver(remoteActorService.ResolveKey),
			),
		)

		_, err = receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireNoActorByAPID(t, db, actorAPID)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
	})

	t.Run("rejects known actor refresh with changed key id", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		oldPublicKey, _, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)
		newPublicKey, newPrivateKey, err := activitypub.GenerateRSAKeyPair()
		require.NoError(t, err)

		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/changed-key-id" {
				http.NotFound(w, r)
				return
			}
			doc := activitypub.ActorDocument(
				"Person",
				server.URL+"/users/changed-key-id",
				"changed-key-id",
				"Changed Key ID",
				"Served with an unexpected key id.",
				newPublicKey,
			)
			doc["publicKey"].(map[string]any)["id"] = server.URL + "/users/changed-key-id#rotated-key"
			rawDoc, err := activitypub.MarshalDocument(doc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/activity+json")
			_, _ = w.Write(rawDoc)
		}))
		defer server.Close()

		actorAPID := server.URL + "/users/changed-key-id"
		remoteActor := createRemoteActorWithPublicKey(t, fx.Ctx, db, actorAPID, "changed-key-id", oldPublicKey)

		activityAPID := server.URL + "/activities/reject-changed-key-id"
		body := []byte(`{"id":"` + activityAPID + `","type":"Follow","actor":"` + actorAPID + `","object":"` + fx.Project.APID + `"}`)
		req := newInboxPostRequest(t, fx.Project.APID, body)
		signRequestWithKey(t, fx.Ctx, req, remoteActor.ID, &httpsig.ActorKey{
			ActorID:       remoteActor.ID,
			ActorAPID:     remoteActor.APID,
			KeyID:         remoteActor.PublicKeyID,
			Algorithm:     httpsig.AlgorithmRSAV15SHA256,
			PublicKeyPEM:  newPublicKey,
			PrivateKeyPEM: newPrivateKey,
		}, body)

		remoteActorService := remoteactor.NewService(
			remoteactor.NewRepository(db),
			remoteactor.WithHTTPClient(server.Client()),
		)
		receiver := remoteinbox.NewService(
			remoteinbox.NewRepository(db, fx.Cfg),
			httpsig.NewService(
				httpsig.NewRepository(db),
				httpsig.WithKeyRefreshResolver(remoteActorService.RefreshKey),
			),
		)

		_, err = receiver.Receive(fx.Ctx, req, fx.Project.APID, body)

		require.ErrorIs(t, err, remoteinbox.ErrUnauthorized)
		requireActorPublicKey(t, db, remoteActor.PublicKeyID, oldPublicKey)
		requireNoActivityByAPID(t, db, activityAPID)
		requireNoInboxActivity(t, db, activityAPID)
		requireNoFollow(t, db, remoteActor.ID, fx.Project.ID)
	})

	t.Run("rejects ticket mutations from non followers without projections", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "not-a-follower")

		cases := []struct {
			name         string
			activityAPID string
			ticketAPID   string
			body         []byte
		}{
			{
				name:         "create ticket",
				activityAPID: "https://remote.example/activities/non-follower-create-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-create",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-create-ticket","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/tickets/non-follower-create","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Blocked remote ticket","content":"Should not land.","forge:priority":"medium","forge:ticketType":"task","forge:isResolved":false}}`),
			},
			{
				name:         "update ticket",
				activityAPID: "https://remote.example/activities/non-follower-update-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-update",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-update-ticket","type":"Update","actor":"` + remoteActor.APID + `","target":"` + fx.Project.APID + `","object":{"id":"https://remote.example/tickets/non-follower-update","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Blocked update"}}`),
			},
			{
				name:         "delete ticket",
				activityAPID: "https://remote.example/activities/non-follower-delete-ticket",
				ticketAPID:   "https://remote.example/tickets/non-follower-delete",
				body:         []byte(`{"id":"https://remote.example/activities/non-follower-delete-ticket","type":"Delete","actor":"` + remoteActor.APID + `","object":"https://remote.example/tickets/non-follower-delete","target":"` + fx.Project.APID + `"}`),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, tc.body)

				_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, tc.body)

				require.ErrorIs(t, err, remoteinbox.ErrForbiddenActor)
				requireNoActivityByAPID(t, db, tc.activityAPID)
				requireNoInboxActivity(t, db, tc.activityAPID)
				requireNoTicketByAPID(t, db, tc.ticketAPID)
				requireNoObjectByAPID(t, db, tc.ticketAPID)
			})
		}
	})

	t.Run("rejects duplicate activity id from another actor", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		firstActor, firstPrivateKey := createRemoteActor(t, fx.Ctx, db, "duplicate-first")
		secondActor, secondPrivateKey := createRemoteActor(t, fx.Ctx, db, "duplicate-second")

		activityAPID := "https://remote.example/activities/reused-activity-id"
		firstObjectAPID := "https://remote.example/objects/reused-first"
		firstBody := []byte(`{"id":"` + activityAPID + `","type":"Create","actor":"` + firstActor.APID + `","object":"` + firstObjectAPID + `"}`)
		firstReq := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, firstActor, firstPrivateKey, firstBody)

		accepted, err := fx.Receiver.Receive(fx.Ctx, firstReq, fx.Project.APID, firstBody)
		require.NoError(t, err)
		require.Equal(t, activityAPID, accepted.ActivityAPID)

		secondBody := []byte(`{"id":"` + activityAPID + `","type":"Create","actor":"` + secondActor.APID + `","object":"https://remote.example/objects/reused-second"}`)
		secondReq := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, secondActor, secondPrivateKey, secondBody)

		_, err = fx.Receiver.Receive(fx.Ctx, secondReq, fx.Project.APID, secondBody)

		require.ErrorIs(t, err, remoteinbox.ErrActivityConflict)
		requireActivityActor(t, db, activityAPID, firstActor.ID)
		requireInboxActivityCount(t, db, activityAPID, 1)
		requireInboxItem(t, db, fx.Project.ID, "Create", firstObjectAPID)
	})

	t.Run("rejects malformed project activities without writes", func(t *testing.T) {
		fx := newInboxIntegrationFixture(t, db)
		remoteActor, privateKey := createRemoteActor(t, fx.Ctx, db, "malformed")

		cases := []struct {
			name         string
			activityAPID string
			body         []byte
		}{
			{
				name:         "follow wrong object",
				activityAPID: "https://remote.example/activities/malformed-follow",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-follow","type":"Follow","actor":"` + remoteActor.APID + `","object":"https://remote.example/projects/other"}`),
			},
			{
				name:         "blank note content",
				activityAPID: "https://remote.example/activities/malformed-note",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-note","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/notes/malformed","type":"Note","attributedTo":"` + remoteActor.APID + `","inReplyTo":"http://localhost:8080/tickets/missing","content":"   "}}`),
			},
			{
				name:         "invalid ticket priority",
				activityAPID: "https://remote.example/activities/malformed-ticket",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-ticket","type":"Create","actor":"` + remoteActor.APID + `","object":{"id":"https://remote.example/tickets/malformed","type":"forge:Ticket","attributedTo":"` + remoteActor.APID + `","context":"` + fx.Project.APID + `","name":"Malformed ticket","forge:priority":"eventually"}}`),
			},
			{
				name:         "add without target",
				activityAPID: "https://remote.example/activities/malformed-add",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-add","type":"Add","actor":"` + remoteActor.APID + `","object":"http://localhost:8080/users/owner"}`),
			},
			{
				name:         "delete project object",
				activityAPID: "https://remote.example/activities/malformed-delete",
				body:         []byte(`{"id":"https://remote.example/activities/malformed-delete","type":"Delete","actor":"` + remoteActor.APID + `","object":"` + fx.Project.APID + `"}`),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := signedRemoteInboxRequest(t, fx.Ctx, fx.Project.APID, remoteActor, privateKey, tc.body)

				_, err := fx.Receiver.Receive(fx.Ctx, req, fx.Project.APID, tc.body)

				require.ErrorIs(t, err, remoteinbox.ErrInvalidActivity)
				requireNoActivityByAPID(t, db, tc.activityAPID)
				requireNoInboxActivity(t, db, tc.activityAPID)
			})
		}
	})
}

type inboxIntegrationFixture struct {
	Ctx      context.Context
	Cfg      activitypub.Config
	Project  *project.Project
	Receiver *remoteinbox.Service
}

func newInboxIntegrationFixture(t *testing.T, db *sqlx.DB) inboxIntegrationFixture {
	t.Helper()

	resetIntegrationDB(t, db)

	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	userService := user.NewService(user.NewRepository(db, cfg), []byte("integration-secret"), cfg)
	projectService := project.NewService(project.NewRepository(db, cfg), cfg)

	owner, err := userService.RegisterUser(ctx, "owner", "owner@example.test", "password123")
	require.NoError(t, err)
	createdProject, err := projectService.CreateProject(ctx, "Inbox Rejection Board", "", owner.ID)
	require.NoError(t, err)

	return inboxIntegrationFixture{
		Ctx:     ctx,
		Cfg:     cfg,
		Project: createdProject,
		Receiver: remoteinbox.NewService(
			remoteinbox.NewRepository(db, cfg),
			httpsig.NewService(httpsig.NewRepository(db)),
		),
	}
}

func createRemoteActor(t *testing.T, ctx context.Context, db *sqlx.DB, username string) (*remoteactor.Actor, string) {
	t.Helper()

	publicKey, privateKey, err := activitypub.GenerateRSAKeyPair()
	require.NoError(t, err)

	actorAPID := "https://remote.example/users/" + username
	actor := createRemoteActorWithPublicKey(t, ctx, db, actorAPID, username, publicKey)
	return actor, privateKey
}

func createRemoteActorWithPublicKey(t *testing.T, ctx context.Context, db *sqlx.DB, actorAPID, username, publicKey string) *remoteactor.Actor {
	t.Helper()

	doc := activitypub.ActorDocument("Person", actorAPID, username, username, "", publicKey)
	rawDoc, err := activitypub.MarshalDocument(doc)
	require.NoError(t, err)

	actor := &remoteactor.Actor{
		APID:              actorAPID,
		Type:              "Person",
		PreferredUsername: username,
		Handle:            username + "@remote.example",
		Name:              username,
		Summary:           "",
		InboxURL:          activitypub.Inbox(actorAPID),
		OutboxURL:         activitypub.Outbox(actorAPID),
		PublicKeyID:       activitypub.KeyID(actorAPID),
		PublicKeyPEM:      publicKey,
		Document:          rawDoc,
	}
	require.NoError(t, remoteactor.NewRepository(db).UpsertRemoteActor(ctx, actor))
	return actor
}

func newInboxPostRequest(t *testing.T, targetAPID string, body []byte) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, targetAPID+"/inbox", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/activity+json")
	return req
}

func signedRemoteInboxRequest(t *testing.T, ctx context.Context, targetAPID string, actor *remoteactor.Actor, privateKey string, body []byte) *http.Request {
	t.Helper()

	req := newInboxPostRequest(t, targetAPID, body)
	signRemoteInboxRequest(t, ctx, req, actor, privateKey, body)
	return req
}

func signRemoteInboxRequest(t *testing.T, ctx context.Context, req *http.Request, actor *remoteactor.Actor, privateKey string, body []byte) {
	t.Helper()

	signRequestWithKey(t, ctx, req, actor.ID, &httpsig.ActorKey{
		ActorID:       actor.ID,
		ActorAPID:     actor.APID,
		KeyID:         actor.PublicKeyID,
		Algorithm:     httpsig.AlgorithmRSAV15SHA256,
		PublicKeyPEM:  actor.PublicKeyPEM,
		PrivateKeyPEM: privateKey,
	}, body)
}

func signRequestWithKey(t *testing.T, ctx context.Context, req *http.Request, actorID string, key *httpsig.ActorKey, body []byte) {
	t.Helper()

	signer := httpsig.NewService(singleKeyRepo{key: key})
	require.NoError(t, signer.SignRequest(ctx, actorID, req, body))
}

type singleKeyRepo struct {
	key *httpsig.ActorKey
}

type recordingDeliveryQueue struct {
	deliveryID  string
	maxAttempts int
	err         error
}

func (q *recordingDeliveryQueue) Enqueue(ctx context.Context, deliveryID string, maxAttempts int) error {
	q.deliveryID = deliveryID
	q.maxAttempts = maxAttempts
	return q.err
}

func (q *recordingDeliveryQueue) Close() error {
	return nil
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

func requireObjectDeleted(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var isDeleted bool
	err := db.Get(&isDeleted, `SELECT is_deleted FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.True(t, isDeleted)
}

func requireNoTicketByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM tickets WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireNoObjectByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM ap_objects WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireNoActorByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM actors WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireActorPublicKey(t *testing.T, db *sqlx.DB, keyID, expectedPublicKey string) {
	t.Helper()

	var publicKey string
	err := db.Get(&publicKey, `SELECT public_key_pem FROM actor_keys WHERE key_id = $1 AND active = true`, keyID)
	require.NoError(t, err)
	require.Equal(t, expectedPublicKey, publicKey)
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

func requireActivityActor(t *testing.T, db *sqlx.DB, apID, expectedActorID string) {
	t.Helper()

	var actorID string
	err := db.Get(&actorID, `SELECT actor_id::text FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Equal(t, expectedActorID, actorID)
}

func requireNoActivityByAPID(t *testing.T, db *sqlx.DB, apID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM ap_activities WHERE ap_id = $1`, apID)
	require.NoError(t, err)
	require.Zero(t, count)
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

func requireActivityForObjectAndTarget(t *testing.T, db *sqlx.DB, activityType, objectAPID, targetAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ap_activities
		WHERE activity_type = $1 AND object_ap_id = $2 AND target_ap_id = $3
	`, activityType, objectAPID, targetAPID)
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

func requireProjectDelivery(t *testing.T, deliveries []delivery.ProjectDelivery, activityType, objectAPID, inboxURL string) {
	t.Helper()

	for _, item := range deliveries {
		if item.ActivityType != activityType || item.TargetInboxURL != inboxURL || item.ObjectAPID == nil {
			continue
		}
		if *item.ObjectAPID == objectAPID {
			return
		}
	}
	t.Fatalf("project delivery not found for %s %s to %s", activityType, objectAPID, inboxURL)
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

func requireNoInboxActivity(t *testing.T, db *sqlx.DB, activityAPID string) {
	t.Helper()

	requireInboxActivityCount(t, db, activityAPID, 0)
}

func requireInboxActivityCount(t *testing.T, db *sqlx.DB, activityAPID string, expected int) {
	t.Helper()

	var count int
	err := db.Get(&count, `SELECT count(*) FROM actor_inbox_items WHERE activity_ap_id = $1`, activityAPID)
	require.NoError(t, err)
	require.Equal(t, expected, count)
}

func requireInboxItemForTarget(t *testing.T, db *sqlx.DB, actorID, activityType, targetAPID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM actor_inbox_items inbox
		JOIN ap_activities activity ON activity.id = inbox.activity_id
		WHERE inbox.actor_id = $1
			AND activity.activity_type = $2
			AND activity.target_ap_id = $3
	`, actorID, activityType, targetAPID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
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

func requireTicketAssignee(t *testing.T, db *sqlx.DB, ticketAPID, assigneeID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ticket_assignees assignee
		JOIN tickets ticket ON ticket.id = assignee.ticket_id
		WHERE ticket.ap_id = $1 AND assignee.actor_id = $2
	`, ticketAPID, assigneeID)
	require.NoError(t, err)
	require.Greater(t, count, 0)
}

func requireNoTicketAssignee(t *testing.T, db *sqlx.DB, ticketAPID, assigneeID string) {
	t.Helper()

	var count int
	err := db.Get(&count, `
		SELECT count(*)
		FROM ticket_assignees assignee
		JOIN tickets ticket ON ticket.id = assignee.ticket_id
		WHERE ticket.ap_id = $1 AND assignee.actor_id = $2
	`, ticketAPID, assigneeID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func requireTicketObjectAssignedTo(t *testing.T, db *sqlx.DB, ticketAPID, assigneeAPID string) {
	t.Helper()

	var assigned bool
	err := db.Get(&assigned, `
		SELECT COALESCE(document->'forge:assignedTo' ? $2, false)
		FROM ap_objects
		WHERE ap_id = $1
	`, ticketAPID, assigneeAPID)
	require.NoError(t, err)
	require.True(t, assigned)
}

func requireTicketObjectNotAssignedTo(t *testing.T, db *sqlx.DB, ticketAPID, assigneeAPID string) {
	t.Helper()

	var assigned bool
	err := db.Get(&assigned, `
		SELECT COALESCE(document->'forge:assignedTo' ? $2, false)
		FROM ap_objects
		WHERE ap_id = $1
	`, ticketAPID, assigneeAPID)
	require.NoError(t, err)
	require.False(t, assigned)
}

func TestIntegrationDBSourceLooksSafe(t *testing.T) {
	source := os.Getenv("TEST_DB_SOURCE")
	if source == "" {
		t.Skip("set TEST_DB_SOURCE to check integration database safety")
	}
	require.True(t, strings.Contains(source, "localhost") || strings.Contains(source, "127.0.0.1") || strings.Contains(source, "db:5432"))
}

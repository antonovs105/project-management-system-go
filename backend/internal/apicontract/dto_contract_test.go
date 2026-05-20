package apicontract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/activitypub/moderation"
	"github.com/antonovs105/project-management-system-go/internal/comment"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/antonovs105/project-management-system-go/internal/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRESTDTOsUseStableSnakeCaseFields(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	assigneeID := "assignee-1"
	parentID := "parent-1"
	lastError := "remote 503"
	nextAttempt := now.Add(time.Minute)
	statusCode := 503

	cases := []struct {
		name       string
		value      any
		wantKeys   []string
		forbidKeys []string
	}{
		{
			name: "user",
			value: user.User{
				ID:            "user-1",
				APID:          "https://local.test/users/alice",
				Username:      "alice",
				Email:         "alice@example.test",
				PasswordHash:  "hash",
				Role:          "user",
				Handle:        "alice@local.test",
				Name:          "Alice",
				Summary:       "Developer",
				PublicKeyPEM:  "public",
				PrivateKeyPEM: "private",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantKeys: []string{"id", "ap_id", "username", "email", "role", "handle", "name", "summary", "created_at", "updated_at"},
			forbidKeys: []string{
				"password_hash", "public_key_pem", "private_key_pem", "passwordHash", "publicKeyPEM", "privateKeyPEM",
			},
		},
		{
			name: "project",
			value: project.Project{
				ID:            "project-1",
				APID:          "https://local.test/projects/project-1",
				Name:          "Board",
				Description:   "Federated board",
				OwnerID:       "user-1",
				Handle:        "project-project-1@local.test",
				PublicKeyPEM:  "public",
				PrivateKeyPEM: "private",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantKeys: []string{"id", "ap_id", "name", "description", "owner_id", "handle", "created_at", "updated_at"},
			forbidKeys: []string{
				"public_key_pem", "private_key_pem", "ownerID", "publicKeyPEM", "privateKeyPEM",
			},
		},
		{
			name: "project invite",
			value: project.ProjectInvite{
				ID:             "invite-1",
				APID:           "https://local.test/activities/invite-1",
				ProjectID:      "project-1",
				InviterActorID: "owner-1",
				InviteeActorID: "invitee-1",
				Role:           "developer",
				Status:         "pending",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			wantKeys:   []string{"id", "ap_id", "project_id", "inviter_actor_id", "invitee_actor_id", "role", "status", "created_at", "updated_at"},
			forbidKeys: []string{"projectID", "inviterActorID", "inviteeActorID"},
		},
		{
			name: "ticket",
			value: ticket.Ticket{
				ID:          "ticket-1",
				APID:        "https://local.test/tickets/ticket-1",
				Title:       "Ship API",
				Description: "Finish backend",
				Status:      "in_progress",
				Priority:    "high",
				Type:        "task",
				ParentID:    &parentID,
				ProjectID:   "project-1",
				ReporterID:  "user-1",
				AssigneeID:  &assigneeID,
				IsResolved:  false,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			wantKeys:   []string{"id", "ap_id", "title", "description", "status", "priority", "type", "parent_id", "project_id", "reporter_id", "assignee_id", "is_resolved", "created_at", "updated_at"},
			forbidKeys: []string{"projectID", "reporterID", "assigneeID", "isResolved"},
		},
		{
			name: "ticket link",
			value: ticket.TicketLink{
				ID:        "link-1",
				SourceID:  "ticket-1",
				TargetID:  "ticket-2",
				LinkType:  "blocks",
				CreatedAt: now,
			},
			wantKeys:   []string{"id", "source_id", "target_id", "link_type", "created_at"},
			forbidKeys: []string{"sourceID", "targetID", "linkType"},
		},
		{
			name: "comment",
			value: comment.Comment{
				ID:        "comment-1",
				APID:      "https://local.test/comments/comment-1",
				TicketID:  "ticket-1",
				AuthorID:  "user-1",
				Content:   "Looks good",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantKeys:   []string{"id", "ap_id", "ticket_id", "author_id", "content", "created_at", "updated_at"},
			forbidKeys: []string{"ticketID", "authorID"},
		},
		{
			name: "delivery",
			value: delivery.Delivery{
				ID:              "delivery-1",
				ActivityID:      "activity-1",
				ActivityAPID:    "https://local.test/activities/activity-1",
				ActorID:         "actor-1",
				ActorAPID:       "https://local.test/users/alice",
				TargetInboxURL:  "https://remote.test/users/bob/inbox",
				State:           delivery.StateFailed,
				Attempts:        2,
				MaxAttempts:     10,
				NextAttemptAt:   &nextAttempt,
				LastError:       &lastError,
				LastAttemptAt:   &now,
				LastFailureKind: delivery.FailureKindHTTP,
				LastStatusCode:  &statusCode,
				Document:        json.RawMessage(`{"should_not":"leak"}`),
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			wantKeys: []string{
				"id", "activity_id", "activity_ap_id", "actor_id", "actor_ap_id", "target_inbox_url",
				"state", "attempts", "max_attempts", "next_attempt_at", "last_error", "last_attempt_at",
				"last_failure_kind", "last_status_code", "created_at", "updated_at",
			},
			forbidKeys: []string{"document", "activityID", "actorAPID", "targetInboxURL", "maxAttempts", "lastFailureKind"},
		},
		{
			name: "federation domain block",
			value: moderation.DomainBlock{
				ID:        "block-1",
				Domain:    "remote.example",
				Reason:    "spam",
				CreatedBy: &assigneeID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantKeys:   []string{"id", "domain", "reason", "created_by", "created_at", "updated_at"},
			forbidKeys: []string{"createdBy"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := marshalObject(t, tc.value)
			for _, key := range tc.wantKeys {
				assert.Contains(t, fields, key)
			}
			for _, key := range tc.forbidKeys {
				assert.NotContains(t, fields, key)
			}
		})
	}
}

func marshalObject(t *testing.T, value any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	return fields
}

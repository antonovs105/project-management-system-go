package apicontract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	apfederation "github.com/antonovs105/project-management-system-go/internal/activitypub/federation"
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
				InstanceRole:  "user",
				Handle:        "alice@local.test",
				Name:          "Alice",
				Summary:       "Developer",
				PublicKeyPEM:  "public",
				PrivateKeyPEM: "private",
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantKeys: []string{"id", "ap_id", "username", "email", "instance_role", "handle", "name", "summary", "created_at", "updated_at"},
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
				RoleID:         "role-1",
				Role:           "developer",
				Status:         "pending",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			wantKeys:   []string{"id", "ap_id", "project_id", "inviter_actor_id", "invitee_actor_id", "role_id", "role", "status", "created_at", "updated_at"},
			forbidKeys: []string{"projectID", "inviterActorID", "inviteeActorID"},
		},
		{
			name: "project member",
			value: project.ProjectMember{
				UserID:    "user-1",
				ProjectID: "project-1",
				RoleID:    "role-1",
				Role:      "developer",
				RoleName:  "Developer",
				Username:  "alice",
				Email:     "alice@example.test",
				Handle:    "alice@local.test",
				Name:      "Alice",
				CreatedAt: now,
			},
			wantKeys:   []string{"user_id", "project_id", "role_id", "role", "role_name", "username", "email", "handle", "name", "created_at"},
			forbidKeys: []string{"userID", "projectID", "roleID", "roleName", "createdAt"},
		},
		{
			name: "project invite inspection",
			value: project.ProjectInviteInspection{
				ID:              "invite-1",
				APID:            "https://local.test/activities/invite-1",
				ProjectID:       "project-1",
				InviterActorID:  "owner-1",
				InviteeActorID:  "invitee-1",
				RoleID:          "role-1",
				Role:            "developer",
				RoleName:        "Developer",
				Status:          "pending",
				InviterUsername: "owner",
				InviterEmail:    "owner@example.test",
				InviterHandle:   "owner@local.test",
				InviterName:     "Owner",
				InviteeUsername: "alice",
				InviteeEmail:    "alice@example.test",
				InviteeHandle:   "alice@local.test",
				InviteeName:     "Alice",
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			wantKeys: []string{
				"id", "ap_id", "project_id", "inviter_actor_id", "invitee_actor_id", "role_id", "role", "role_name",
				"status", "inviter_username", "inviter_email", "inviter_handle", "inviter_name",
				"invitee_username", "invitee_email", "invitee_handle", "invitee_name", "created_at", "updated_at",
			},
			forbidKeys: []string{"projectID", "roleName", "inviterUsername", "inviteeUsername"},
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
		{
			name: "federation remote actor",
			value: moderation.RemoteActorInspection{
				ID:                "actor-1",
				APID:              "https://remote.test/users/alice",
				Type:              "Person",
				PreferredUsername: "alice",
				Handle:            "alice@remote.test",
				Name:              "Alice",
				Summary:           "Remote",
				InboxURL:          "https://remote.test/users/alice/inbox",
				OutboxURL:         "https://remote.test/users/alice/outbox",
				LastFetchedAt:     &now,
				FetchError:        &lastError,
				FetchErrorAt:      &nextAttempt,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
			wantKeys: []string{
				"id", "ap_id", "type", "preferred_username", "handle", "name", "summary",
				"inbox_url", "outbox_url", "last_fetched_at", "fetch_error", "fetch_error_at", "created_at", "updated_at",
			},
			forbidKeys: []string{"public_key_pem", "document", "preferredUsername", "inboxURL", "fetchErrorAt"},
		},
		{
			name: "federation delivery inspection",
			value: moderation.FederationDeliveryInspection{
				ID:              "delivery-1",
				ActivityAPID:    "https://local.test/activities/activity-1",
				ActivityType:    "Create",
				ActorAPID:       "https://local.test/users/alice",
				ProjectID:       &parentID,
				ProjectAPID:     &lastError,
				TargetInboxURL:  "https://remote.test/users/bob/inbox",
				State:           delivery.StateDead,
				Attempts:        2,
				MaxAttempts:     10,
				LastFailureKind: delivery.FailureKindHTTP,
				LastStatusCode:  &statusCode,
				CanRetry:        true,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
			wantKeys: []string{
				"id", "activity_ap_id", "activity_type", "actor_ap_id", "project_id", "project_ap_id",
				"target_inbox_url", "state", "attempts", "max_attempts", "last_failure_kind", "last_status_code",
				"can_retry", "created_at", "updated_at",
			},
			forbidKeys: []string{"activityAPID", "actorAPID", "targetInboxURL", "canRetry"},
		},
		{
			name: "federation delivery summary",
			value: moderation.FederationDeliverySummary{
				Total:           5,
				Pending:         1,
				Processing:      1,
				Delivered:       1,
				Failed:          1,
				Dead:            1,
				Retryable:       2,
				DueRetry:        1,
				HTTPFailures:    1,
				NetworkFailures: 1,
				SigningFailures: 1,
				SafetyFailures:  1,
				UnknownFailures: 1,
				OldestPendingAt: &now,
				OldestDeadAt:    &nextAttempt,
				CanRetry:        true,
			},
			wantKeys: []string{
				"total", "pending", "processing", "delivered", "failed", "dead", "retryable", "due_retry",
				"http_failures", "network_failures", "signing_failures", "safety_failures", "unknown_failures",
				"oldest_pending_at", "oldest_dead_at", "can_retry",
			},
			forbidKeys: []string{"dueRetry", "httpFailures", "oldestDeadAt", "canRetry"},
		},
		{
			name: "personal federation inbox activity",
			value: apfederation.InboxActivity{
				ID:            "activity-1",
				ActivityAPID:  "https://remote.test/activities/1",
				ActivityType:  "Create",
				ActorID:       "actor-1",
				ActorAPID:     "https://remote.test/users/alice",
				ActorType:     "Person",
				ActorHandle:   "alice@remote.test",
				ActorName:     "Alice",
				ObjectAPID:    &parentID,
				ObjectType:    &lastError,
				ObjectName:    &assigneeID,
				ObjectContent: &lastError,
				TargetAPID:    &parentID,
				TargetActorID: &assigneeID,
				TargetType:    &lastError,
				TargetHandle:  &lastError,
				TargetName:    &lastError,
				ReceivedAt:    now,
				CreatedAt:     now,
			},
			wantKeys: []string{
				"id", "activity_ap_id", "activity_type", "actor_id", "actor_ap_id", "actor_type",
				"actor_handle", "actor_name", "object_ap_id", "object_type", "object_name",
				"object_content", "target_ap_id", "target_actor_id", "target_type",
				"target_handle", "target_name", "received_at", "created_at",
			},
			forbidKeys: []string{"activityAPID", "actorAPID", "objectAPID", "targetActorID", "receivedAt"},
		},
		{
			name: "personal federation remote follow",
			value: apfederation.RemoteFollow{
				ActorID:           "actor-1",
				ActorAPID:         "https://remote.test/projects/board",
				ActorType:         "Group",
				PreferredUsername: "board",
				Handle:            "board@remote.test",
				Name:              "Remote Board",
				Summary:           "Remote project",
				InboxURL:          "https://remote.test/projects/board/inbox",
				OutboxURL:         "https://remote.test/projects/board/outbox",
				FollowersURL:      &parentID,
				FollowingURL:      &assigneeID,
				State:             "accepted",
				CreatedAt:         now,
				UpdatedAt:         now,
			},
			wantKeys: []string{
				"actor_id", "actor_ap_id", "actor_type", "preferred_username", "handle",
				"name", "summary", "inbox_url", "outbox_url", "followers_url",
				"following_url", "state", "created_at", "updated_at",
			},
			forbidKeys: []string{"actorID", "actorAPID", "preferredUsername", "inboxURL", "followersURL"},
		},
		{
			name: "personal federation remote actor",
			value: apfederation.RemoteActor{
				ID:                "actor-1",
				APID:              "https://remote.test/projects/board",
				Type:              "Group",
				PreferredUsername: "board",
				Handle:            "board@remote.test",
				Name:              "Remote Board",
				Summary:           "Remote project",
				InboxURL:          "https://remote.test/projects/board/inbox",
				OutboxURL:         "https://remote.test/projects/board/outbox",
				FollowersURL:      &parentID,
				FollowingURL:      &assigneeID,
				LastFetchedAt:     &now,
				CreatedAt:         now,
				UpdatedAt:         now,
			},
			wantKeys: []string{
				"id", "ap_id", "type", "preferred_username", "handle", "name", "summary",
				"inbox_url", "outbox_url", "followers_url", "following_url",
				"last_fetched_at", "created_at", "updated_at",
			},
			forbidKeys: []string{"public_key_pem", "document", "preferredUsername", "inboxURL", "lastFetchedAt"},
		},
		{
			name: "personal federation follow result",
			value: apfederation.FollowRemoteActorResult{
				Follow: apfederation.RemoteFollow{
					ActorID:           "actor-1",
					ActorAPID:         "https://remote.test/projects/board",
					ActorType:         "Group",
					PreferredUsername: "board",
					Handle:            "board@remote.test",
					Name:              "Remote Board",
					Summary:           "Remote project",
					InboxURL:          "https://remote.test/projects/board/inbox",
					OutboxURL:         "https://remote.test/projects/board/outbox",
					State:             "pending",
					CreatedAt:         now,
					UpdatedAt:         now,
				},
				Delivery: &apfederation.FollowDelivery{
					ID:             "delivery-1",
					ActivityAPID:   "https://local.test/activities/activity-1",
					TargetInboxURL: "https://remote.test/projects/board/inbox",
					State:          delivery.StatePending,
					CreatedAt:      now,
					UpdatedAt:      now,
				},
				Created: true,
			},
			wantKeys:   []string{"follow", "delivery", "created"},
			forbidKeys: []string{"actorID", "targetInboxURL", "activityAPID", "Created"},
		},
		{
			name: "personal federation follow delivery",
			value: apfederation.FollowDelivery{
				ID:             "delivery-1",
				ActivityAPID:   "https://local.test/activities/activity-1",
				TargetInboxURL: "https://remote.test/projects/board/inbox",
				State:          delivery.StatePending,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			wantKeys:   []string{"id", "activity_ap_id", "target_inbox_url", "state", "created_at", "updated_at"},
			forbidKeys: []string{"activityAPID", "targetInboxURL", "createdAt"},
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

package activitypub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorDocuments(t *testing.T) {
	user := ActorDocument("Person", "https://example.test/users/alice", "alice", "Alice", "", "public-key")
	assert.Equal(t, "Person", user["type"])
	assert.Equal(t, "https://example.test/users/alice/inbox", user["inbox"])
	assert.Equal(t, "https://example.test/users/alice/outbox", user["outbox"])
	assert.Contains(t, user, "publicKey")

	project := ProjectActorDocument("https://example.test/projects/project-1", "Project One", "Summary", "public-key")
	assert.Equal(t, []string{"Group", "forge:Project", "forge:TicketTracker"}, project["type"])
	assert.Equal(t, "https://example.test/projects/project-1/tickets", project["tickets"])
}

func TestTicketNoteAndActivityDocuments(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	parent := "https://example.test/tickets/parent"

	ticket := TicketDocument(
		"https://example.test/tickets/ticket-1",
		"https://example.test/projects/project-1",
		"https://example.test/users/alice",
		"Fix login",
		"Make login work",
		"done",
		"high",
		"task",
		&parent,
		[]string{"https://example.test/users/bob"},
		now,
		true,
	)
	assert.Equal(t, "forge:Ticket", ticket["type"])
	assert.Equal(t, true, ticket["forge:isResolved"])
	assert.Equal(t, parent, ticket["inReplyTo"])
	assert.Equal(t, []string{"https://example.test/users/bob"}, ticket["forge:assignedTo"])

	note := NoteDocument("https://example.test/comments/comment-1", ticket["id"].(string), "https://example.test/users/alice", "Done", now)
	assert.Equal(t, "Note", note["type"])
	assert.Equal(t, ticket["id"], note["inReplyTo"])

	create := ActivityDocument("Create", "https://example.test/activities/activity-1", "https://example.test/users/alice", ticket, nil, now)
	assert.Equal(t, "Create", create["type"])
	assert.Equal(t, ticket, create["object"])

	raw, err := MarshalDocument(create)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "https://www.w3.org/ns/activitystreams")
}

package ticket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventHubPublishesProjectEvents(t *testing.T) {
	hub := NewEventHub()
	events, unsubscribe := hub.SubscribeTicketEvents("project-1")
	defer unsubscribe()

	hub.PublishTicketEvent(Event{Type: EventTicketUpdated, ProjectID: "project-1", TicketID: "ticket-1"})

	select {
	case event := <-events:
		assert.Equal(t, EventTicketUpdated, event.Type)
		assert.Equal(t, "project-1", event.ProjectID)
		assert.Equal(t, "ticket-1", event.TicketID)
		assert.NotEmpty(t, event.ID)
		assert.False(t, event.OccurredAt.IsZero())
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for ticket event")
	}
}

func TestEventHubScopesSubscribersByProject(t *testing.T) {
	hub := NewEventHub()
	events, unsubscribe := hub.SubscribeTicketEvents("project-1")
	defer unsubscribe()

	hub.PublishTicketEvent(Event{Type: EventTicketUpdated, ProjectID: "project-2", TicketID: "ticket-1"})

	select {
	case event := <-events:
		require.Failf(t, "unexpected event", "received %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEventHubUnsubscribeIsIdempotent(t *testing.T) {
	hub := NewEventHub()
	_, unsubscribe := hub.SubscribeTicketEvents("project-1")

	unsubscribe()

	assert.NotPanics(t, unsubscribe)
	assert.NotPanics(t, func() {
		hub.PublishTicketEvent(Event{Type: EventTicketUpdated, ProjectID: "project-1", TicketID: "ticket-1"})
	})
}

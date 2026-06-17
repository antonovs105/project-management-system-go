package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventHubPublishesUserNotifications(t *testing.T) {
	hub := NewEventHub()
	events, unsubscribe := hub.SubscribeNotifications("user-1")
	defer unsubscribe()

	hub.PublishNotification(Notification{ID: "n-1", UserID: "user-1", Type: TypeTicketAssigned})

	select {
	case event := <-events:
		assert.Equal(t, "n-1", event.ID)
		assert.Equal(t, "user-1", event.UserID)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for notification")
	}
}

func TestEventHubScopesNotificationsByUser(t *testing.T) {
	hub := NewEventHub()
	events, unsubscribe := hub.SubscribeNotifications("user-1")
	defer unsubscribe()

	hub.PublishNotification(Notification{ID: "n-1", UserID: "user-2", Type: TypeTicketAssigned})

	select {
	case event := <-events:
		require.Failf(t, "unexpected event", "received %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

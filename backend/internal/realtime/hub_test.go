package realtime

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type testEvent struct {
	ID  string
	Key string
}

func testConfig() Config[testEvent] {
	return Config[testEvent]{
		Channel: "test",
		Key:     func(value testEvent) string { return value.Key },
		ID:      func(value testEvent) string { return value.ID },
	}
}

func TestLocalHubScopesAndRetainsNewestBufferedValues(t *testing.T) {
	hub := NewLocalHub(testConfig())
	values, unsubscribe := hub.Subscribe("project-1")
	defer unsubscribe()
	for index := 1; index <= 20; index++ {
		hub.Publish(testEvent{ID: fmt.Sprint(index), Key: "project-1"})
		hub.Publish(testEvent{ID: "other", Key: "project-2"})
	}
	first := <-values
	require.Equal(t, "5", first.ID)
	for index := 6; index <= 20; index++ {
		require.Equal(t, fmt.Sprint(index), (<-values).ID)
	}
}

func TestRedisHubSuppressesLocalEchoesWithoutClient(t *testing.T) {
	hub := NewRedisHub[testEvent](nil, testConfig())
	defer hub.Close()
	values, unsubscribe := hub.Subscribe("user-1")
	defer unsubscribe()
	event := testEvent{ID: "event-1", Key: "user-1"}
	hub.PublishLocalOnce(event)
	hub.PublishLocalOnce(event)
	require.Equal(t, event, <-values)
	select {
	case duplicate := <-values:
		require.Failf(t, "unexpected duplicate", "%#v", duplicate)
	default:
	}
}

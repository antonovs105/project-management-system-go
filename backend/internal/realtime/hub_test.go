package realtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

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

func TestLocalHubSustainsBoundedConcurrentFanout(t *testing.T) {
	const subscriberCount = 64
	const publisherCount = 16
	const valuesPerPublisher = 500

	hub := NewLocalHub(testConfig())
	unsubscribes := make([]func(), 0, subscriberCount)
	for index := 0; index < subscriberCount; index++ {
		_, unsubscribe := hub.Subscribe("project-1")
		unsubscribes = append(unsubscribes, unsubscribe)
	}

	done := make(chan struct{})
	go func() {
		var publishers sync.WaitGroup
		publishers.Add(publisherCount)
		for publisher := 0; publisher < publisherCount; publisher++ {
			go func(publisher int) {
				defer publishers.Done()
				for index := 0; index < valuesPerPublisher; index++ {
					hub.Publish(testEvent{ID: fmt.Sprintf("%d-%d", publisher, index), Key: "project-1"})
				}
			}(publisher)
		}
		publishers.Wait()
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("bounded fanout blocked under concurrent publication")
	}
	for _, unsubscribe := range unsubscribes {
		unsubscribe()
		unsubscribe()
	}
}

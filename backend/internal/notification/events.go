package notification

import (
	"github.com/antonovs105/project-management-system-go/internal/realtime"
	"github.com/redis/go-redis/v9"
)

// redisNotificationEventsChannel carries notification events between API replicas.
const redisNotificationEventsChannel = "progo:events:notifications"

// EventPublisher receives notifications after they are persisted.
type EventPublisher interface {
	PublishNotification(Notification)
}

// EventSubscriber exposes per-user notification streams.
type EventSubscriber interface {
	SubscribeNotifications(userID string) (<-chan Notification, func())
}

// EventHub is an in-memory fanout for local notification events.
type EventHub struct {
	hub *realtime.LocalHub[Notification]
}

// NewEventHub creates an in-process notification fanout.
func NewEventHub() *EventHub {
	return &EventHub{hub: realtime.NewLocalHub(notificationEventConfig())}
}

// PublishNotification fans out one notification to the owning user.
func (h *EventHub) PublishNotification(item Notification) {
	if h != nil && h.hub != nil {
		h.hub.Publish(item)
	}
}

// SubscribeNotifications subscribes to notifications for one user.
func (h *EventHub) SubscribeNotifications(userID string) (<-chan Notification, func()) {
	if h == nil || h.hub == nil {
		return NewEventHub().SubscribeNotifications(userID)
	}
	return h.hub.Subscribe(userID)
}

// RedisEventHub distributes notification events across API replicas using Redis Pub/Sub.
type RedisEventHub struct {
	hub *realtime.RedisHub[Notification]
}

// NewRedisEventHub creates a Redis-backed notification fanout.
func NewRedisEventHub(client *redis.Client) *RedisEventHub {
	return &RedisEventHub{hub: realtime.NewRedisHub(client, notificationEventConfig())}
}

// PublishNotification fans out locally and publishes the notification for other API replicas.
func (h *RedisEventHub) PublishNotification(item Notification) {
	if h != nil && h.hub != nil {
		h.hub.Publish(item)
	}
}

// SubscribeNotifications subscribes to notifications for one user.
func (h *RedisEventHub) SubscribeNotifications(userID string) (<-chan Notification, func()) {
	if h == nil || h.hub == nil {
		return NewEventHub().SubscribeNotifications(userID)
	}
	return h.hub.Subscribe(userID)
}

// Close stops the Redis subscription loop.
func (h *RedisEventHub) Close() error {
	if h == nil || h.hub == nil {
		return nil
	}
	return h.hub.Close()
}

// publishLocalOnce is retained as a focused duplicate-suppression test seam.
func (h *RedisEventHub) publishLocalOnce(item Notification) {
	if h != nil && h.hub != nil {
		h.hub.PublishLocalOnce(item)
	}
}

// notificationEventConfig defines notification channel routing.
func notificationEventConfig() realtime.Config[Notification] {
	return realtime.Config[Notification]{
		Channel:   redisNotificationEventsChannel,
		LogPrefix: "notification_event",
		Key:       func(item Notification) string { return item.UserID },
		ID:        func(item Notification) string { return item.ID },
	}
}

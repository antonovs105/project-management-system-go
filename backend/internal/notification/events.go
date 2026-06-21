package notification

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// redisNotificationEventsChannel carries notification events between API replicas.
	redisNotificationEventsChannel = "progo:events:notifications"
	// eventSeenTTL bounds duplicate suppression memory for Redis echo messages.
	eventSeenTTL = 5 * time.Minute
	// redisSubscribeRetryDelay spaces reconnect attempts after subscription failures.
	redisSubscribeRetryDelay = 2 * time.Second
)

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
	mu          sync.RWMutex
	subscribers map[string]map[chan Notification]struct{}
}

// NewEventHub creates an in-process notification fanout.
func NewEventHub() *EventHub {
	return &EventHub{subscribers: make(map[string]map[chan Notification]struct{})}
}

// PublishNotification fans out one notification to the owning user.
func (h *EventHub) PublishNotification(item Notification) {
	if h == nil || item.UserID == "" {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers[item.UserID] {
		select {
		case subscriber <- item:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- item:
			default:
			}
		}
	}
}

// SubscribeNotifications subscribes to notifications for one user.
func (h *EventHub) SubscribeNotifications(userID string) (<-chan Notification, func()) {
	ch := make(chan Notification, 16)
	if h == nil || userID == "" {
		var once sync.Once
		return ch, func() {
			once.Do(func() {
				close(ch)
			})
		}
	}

	h.mu.Lock()
	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[chan Notification]struct{})
	}
	h.subscribers[userID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if userSubscribers := h.subscribers[userID]; userSubscribers != nil {
				delete(userSubscribers, ch)
				if len(userSubscribers) == 0 {
					delete(h.subscribers, userID)
				}
			}
			close(ch)
		})
	}
	return ch, unsubscribe
}

// RedisEventHub distributes notification events across API replicas using Redis Pub/Sub.
type RedisEventHub struct {
	local  *EventHub
	client *redis.Client

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu   sync.Mutex
	seen map[string]time.Time
}

// NewRedisEventHub creates a Redis-backed notification fanout.
func NewRedisEventHub(client *redis.Client) *RedisEventHub {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &RedisEventHub{
		local:  NewEventHub(),
		client: client,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		seen:   make(map[string]time.Time),
	}
	if client == nil {
		close(hub.done)
		return hub
	}
	go hub.subscribe()
	return hub
}

// PublishNotification fans out locally and publishes the notification for other API replicas.
func (h *RedisEventHub) PublishNotification(item Notification) {
	if h == nil || item.UserID == "" {
		return
	}
	h.publishLocalOnce(item)
	if h.client == nil {
		return
	}

	raw, err := json.Marshal(item)
	if err != nil {
		log.Printf("notification_event_marshal_failed notification_id=%s user_id=%s error=%v", item.ID, item.UserID, err)
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, 500*time.Millisecond)
	defer cancel()
	if err := h.client.Publish(ctx, redisNotificationEventsChannel, raw).Err(); err != nil && h.ctx.Err() == nil {
		log.Printf("notification_event_publish_failed notification_id=%s user_id=%s error=%v", item.ID, item.UserID, err)
	}
}

// SubscribeNotifications subscribes to notifications for one user.
func (h *RedisEventHub) SubscribeNotifications(userID string) (<-chan Notification, func()) {
	if h == nil || h.local == nil {
		return NewEventHub().SubscribeNotifications(userID)
	}
	return h.local.SubscribeNotifications(userID)
}

// Close stops the Redis subscription loop.
func (h *RedisEventHub) Close() error {
	if h == nil {
		return nil
	}
	h.cancel()
	<-h.done
	return nil
}

// subscribe copies Redis notification messages into the local event hub.
func (h *RedisEventHub) subscribe() {
	defer close(h.done)
	for {
		if h.ctx.Err() != nil {
			return
		}
		pubsub := h.client.Subscribe(h.ctx, redisNotificationEventsChannel)
		if _, err := pubsub.Receive(h.ctx); err != nil {
			_ = pubsub.Close()
			if h.ctx.Err() != nil {
				return
			}
			log.Printf("notification_event_subscribe_failed error=%v", err)
			select {
			case <-time.After(redisSubscribeRetryDelay):
			case <-h.ctx.Done():
				return
			}
			continue
		}

		for message := range pubsub.Channel() {
			var item Notification
			if err := json.Unmarshal([]byte(message.Payload), &item); err != nil {
				log.Printf("notification_event_unmarshal_failed error=%v", err)
				continue
			}
			if item.UserID == "" {
				continue
			}
			h.publishLocalOnce(item)
		}
		_ = pubsub.Close()
	}
}

// publishLocalOnce fans out one notification locally while suppressing Redis echoes.
func (h *RedisEventHub) publishLocalOnce(item Notification) {
	if item.ID == "" {
		h.local.PublishNotification(item)
		return
	}
	now := time.Now().UTC()
	h.mu.Lock()
	for id, expiresAt := range h.seen {
		if !expiresAt.After(now) {
			delete(h.seen, id)
		}
	}
	if _, ok := h.seen[item.ID]; ok {
		h.mu.Unlock()
		return
	}
	h.seen[item.ID] = now.Add(eventSeenTTL)
	h.mu.Unlock()
	h.local.PublishNotification(item)
}

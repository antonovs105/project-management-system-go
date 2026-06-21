package ticket

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// EventTicketCreated is emitted after a ticket is created.
	EventTicketCreated = "ticket.created"
	// EventTicketUpdated is emitted after a ticket changes.
	EventTicketUpdated = "ticket.updated"
	// EventTicketDeleted is emitted after a ticket is deleted.
	EventTicketDeleted = "ticket.deleted"
	// EventTicketLinked is emitted after a ticket relationship changes.
	EventTicketLinked = "ticket.linked"
	// EventTicketUnlinked is emitted after a ticket relationship is removed.
	EventTicketUnlinked = "ticket.unlinked"

	// redisTicketEventsChannel carries ticket events between API replicas.
	redisTicketEventsChannel = "progo:events:tickets"
	// eventSeenTTL bounds duplicate suppression memory for Redis echo messages.
	eventSeenTTL = 5 * time.Minute
	// redisSubscribeRetryDelay spaces reconnect attempts after subscription failures.
	redisSubscribeRetryDelay = 2 * time.Second
)

// Event describes a project ticket change for realtime clients.
type Event struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	ProjectID  string    `json:"project_id"`
	TicketID   string    `json:"ticket_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventPublisher receives ticket events from write workflows.
type EventPublisher interface {
	PublishTicketEvent(Event)
}

// EventSubscriber exposes project-scoped ticket event streams.
type EventSubscriber interface {
	SubscribeTicketEvents(projectID string) (<-chan Event, func())
}

// EventHub is an in-memory fanout for ticket events inside one API process.
type EventHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

// NewEventHub creates an in-process ticket event fanout.
func NewEventHub() *EventHub {
	return &EventHub{subscribers: make(map[string]map[chan Event]struct{})}
}

// PublishTicketEvent fans out a ticket event to subscribers of its project.
func (h *EventHub) PublishTicketEvent(event Event) {
	if h == nil || event.ProjectID == "" {
		return
	}
	event = normalizeTicketEvent(event)

	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers[event.ProjectID] {
		select {
		case subscriber <- event:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- event:
			default:
			}
		}
	}
}

// SubscribeTicketEvents subscribes to ticket events for one project.
func (h *EventHub) SubscribeTicketEvents(projectID string) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	if h == nil || projectID == "" {
		var once sync.Once
		return ch, func() {
			once.Do(func() {
				close(ch)
			})
		}
	}

	h.mu.Lock()
	if h.subscribers[projectID] == nil {
		h.subscribers[projectID] = make(map[chan Event]struct{})
	}
	h.subscribers[projectID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if projectSubscribers := h.subscribers[projectID]; projectSubscribers != nil {
				delete(projectSubscribers, ch)
				if len(projectSubscribers) == 0 {
					delete(h.subscribers, projectID)
				}
			}
			close(ch)
		})
	}
	return ch, unsubscribe
}

// RedisEventHub distributes ticket events across API replicas using Redis Pub/Sub.
type RedisEventHub struct {
	local  *EventHub
	client *redis.Client

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu   sync.Mutex
	seen map[string]time.Time
}

// NewRedisEventHub creates a Redis-backed ticket event fanout.
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

// PublishTicketEvent fans out locally and publishes the event for other API replicas.
func (h *RedisEventHub) PublishTicketEvent(event Event) {
	if h == nil || event.ProjectID == "" {
		return
	}
	event = normalizeTicketEvent(event)
	h.publishLocalOnce(event)
	if h.client == nil {
		return
	}

	raw, err := json.Marshal(event)
	if err != nil {
		log.Printf("ticket_event_marshal_failed event_id=%s project_id=%s error=%v", event.ID, event.ProjectID, err)
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, 500*time.Millisecond)
	defer cancel()
	if err := h.client.Publish(ctx, redisTicketEventsChannel, raw).Err(); err != nil && h.ctx.Err() == nil {
		log.Printf("ticket_event_publish_failed event_id=%s project_id=%s error=%v", event.ID, event.ProjectID, err)
	}
}

// SubscribeTicketEvents subscribes to ticket events for one project.
func (h *RedisEventHub) SubscribeTicketEvents(projectID string) (<-chan Event, func()) {
	if h == nil || h.local == nil {
		return NewEventHub().SubscribeTicketEvents(projectID)
	}
	return h.local.SubscribeTicketEvents(projectID)
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

// subscribe copies Redis ticket messages into the local event hub.
func (h *RedisEventHub) subscribe() {
	defer close(h.done)
	for {
		if h.ctx.Err() != nil {
			return
		}
		pubsub := h.client.Subscribe(h.ctx, redisTicketEventsChannel)
		if _, err := pubsub.Receive(h.ctx); err != nil {
			_ = pubsub.Close()
			if h.ctx.Err() != nil {
				return
			}
			log.Printf("ticket_event_subscribe_failed error=%v", err)
			select {
			case <-time.After(redisSubscribeRetryDelay):
			case <-h.ctx.Done():
				return
			}
			continue
		}

		for message := range pubsub.Channel() {
			var event Event
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				log.Printf("ticket_event_unmarshal_failed error=%v", err)
				continue
			}
			if event.ProjectID == "" {
				continue
			}
			h.publishLocalOnce(normalizeTicketEvent(event))
		}
		_ = pubsub.Close()
	}
}

// publishLocalOnce fans out one ticket event locally while suppressing Redis echoes.
func (h *RedisEventHub) publishLocalOnce(event Event) {
	if event.ID == "" {
		h.local.PublishTicketEvent(event)
		return
	}
	now := time.Now().UTC()
	h.mu.Lock()
	for id, expiresAt := range h.seen {
		if !expiresAt.After(now) {
			delete(h.seen, id)
		}
	}
	if _, ok := h.seen[event.ID]; ok {
		h.mu.Unlock()
		return
	}
	h.seen[event.ID] = now.Add(eventSeenTTL)
	h.mu.Unlock()
	h.local.PublishTicketEvent(event)
}

// normalizeTicketEvent fills generated metadata for realtime ticket events.
func normalizeTicketEvent(event Event) Event {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	return event
}

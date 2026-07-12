package ticket

import (
	"time"

	"github.com/antonovs105/project-management-system-go/internal/realtime"
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
type EventHub struct{ hub *realtime.LocalHub[Event] }

// NewEventHub creates an in-process ticket event fanout.
func NewEventHub() *EventHub {
	return &EventHub{hub: realtime.NewLocalHub(ticketEventConfig())}
}

// PublishTicketEvent fans out a ticket event to subscribers of its project.
func (h *EventHub) PublishTicketEvent(event Event) {
	if h != nil && h.hub != nil {
		h.hub.Publish(event)
	}
}

// SubscribeTicketEvents subscribes to ticket events for one project.
func (h *EventHub) SubscribeTicketEvents(projectID string) (<-chan Event, func()) {
	if h == nil || h.hub == nil {
		return NewEventHub().SubscribeTicketEvents(projectID)
	}
	return h.hub.Subscribe(projectID)
}

// RedisEventHub distributes ticket events across API replicas using Redis Pub/Sub.
type RedisEventHub struct{ hub *realtime.RedisHub[Event] }

// NewRedisEventHub creates a Redis-backed ticket event fanout.
func NewRedisEventHub(client *redis.Client) *RedisEventHub {
	return &RedisEventHub{hub: realtime.NewRedisHub(client, ticketEventConfig())}
}

// PublishTicketEvent fans out locally and publishes the event for other API replicas.
func (h *RedisEventHub) PublishTicketEvent(event Event) {
	if h != nil && h.hub != nil {
		h.hub.Publish(event)
	}
}

// SubscribeTicketEvents subscribes to ticket events for one project.
func (h *RedisEventHub) SubscribeTicketEvents(projectID string) (<-chan Event, func()) {
	if h == nil || h.hub == nil {
		return NewEventHub().SubscribeTicketEvents(projectID)
	}
	return h.hub.Subscribe(projectID)
}

// Close stops the Redis subscription loop.
func (h *RedisEventHub) Close() error {
	if h == nil || h.hub == nil {
		return nil
	}
	return h.hub.Close()
}

// publishLocalOnce is retained as a focused duplicate-suppression test seam.
func (h *RedisEventHub) publishLocalOnce(event Event) {
	if h != nil && h.hub != nil {
		h.hub.PublishLocalOnce(event)
	}
}

// ticketEventConfig defines ticket channel routing and normalization.
func ticketEventConfig() realtime.Config[Event] {
	return realtime.Config[Event]{
		Channel:   redisTicketEventsChannel,
		LogPrefix: "ticket_event",
		Key:       func(event Event) string { return event.ProjectID },
		ID:        func(event Event) string { return event.ID },
		Normalize: normalizeTicketEvent,
	}
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

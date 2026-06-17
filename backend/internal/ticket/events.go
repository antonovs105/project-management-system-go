package ticket

import (
	"sync"
	"time"

	"github.com/google/uuid"
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
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

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

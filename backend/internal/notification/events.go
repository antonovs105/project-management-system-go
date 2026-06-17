package notification

import (
	"sync"
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

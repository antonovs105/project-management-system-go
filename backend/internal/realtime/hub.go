// Package realtime provides a small typed local and Redis pub/sub fanout.
package realtime

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Internal bounds keep fanout non-blocking and Redis recovery bounded.
const (
	// defaultBufferSize bounds each slow subscriber's retained values.
	defaultBufferSize = 16
	// eventSeenTTL bounds duplicate suppression memory for Redis echoes.
	eventSeenTTL = 5 * time.Minute
	// redisSubscribeRetryDelay spaces reconnect attempts after subscription failures.
	redisSubscribeRetryDelay = 2 * time.Second
	// redisPublishTimeout prevents event publication from delaying write workflows.
	redisPublishTimeout = 500 * time.Millisecond
)

// Config describes one typed realtime channel.
type Config[T any] struct {
	Channel   string
	LogPrefix string
	Key       func(T) string
	ID        func(T) string
	Normalize func(T) T
}

// LocalHub fans typed values out to subscribers sharing the same key.
type LocalHub[T any] struct {
	config      Config[T]
	mu          sync.RWMutex
	subscribers map[string]map[chan T]struct{}
}

// NewLocalHub returns an in-process typed fanout.
func NewLocalHub[T any](config Config[T]) *LocalHub[T] {
	return &LocalHub[T]{config: config, subscribers: make(map[string]map[chan T]struct{})}
}

// Publish delivers a normalized value to subscribers of its key. Slow
// subscribers retain the newest value instead of blocking writers.
func (h *LocalHub[T]) Publish(value T) {
	if h == nil {
		return
	}
	value = normalize(h.config, value)
	key := h.config.Key(value)
	if key == "" {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers[key] {
		select {
		case subscriber <- value:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- value:
			default:
			}
		}
	}
}

// Subscribe returns a bounded stream and idempotent unsubscribe function.
func (h *LocalHub[T]) Subscribe(key string) (<-chan T, func()) {
	values := make(chan T, defaultBufferSize)
	if h == nil || key == "" {
		var once sync.Once
		return values, func() { once.Do(func() { close(values) }) }
	}
	h.mu.Lock()
	if h.subscribers[key] == nil {
		h.subscribers[key] = make(map[chan T]struct{})
	}
	h.subscribers[key][values] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return values, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subscribers[key], values)
			if len(h.subscribers[key]) == 0 {
				delete(h.subscribers, key)
			}
			close(values)
		})
	}
}

// RedisHub extends LocalHub across replicas using Redis Pub/Sub.
type RedisHub[T any] struct {
	config Config[T]
	local  *LocalHub[T]
	client *redis.Client
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	seen   map[string]time.Time
}

// NewRedisHub creates a typed Redis-backed fanout. A nil client retains local behavior.
func NewRedisHub[T any](client *redis.Client, config Config[T]) *RedisHub[T] {
	ctx, cancel := context.WithCancel(context.Background())
	hub := &RedisHub[T]{
		config: config,
		local:  NewLocalHub(config),
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

// Publish fans out locally and publishes to other replicas.
func (h *RedisHub[T]) Publish(value T) {
	if h == nil {
		return
	}
	value = normalize(h.config, value)
	if h.config.Key(value) == "" {
		return
	}
	h.PublishLocalOnce(value)
	if h.client == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		h.log("marshal_failed", err)
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, redisPublishTimeout)
	defer cancel()
	if err := h.client.Publish(ctx, h.config.Channel, raw).Err(); err != nil && h.ctx.Err() == nil {
		h.log("publish_failed", err)
	}
}

// Subscribe returns a local stream fed by this process and remote replicas.
func (h *RedisHub[T]) Subscribe(key string) (<-chan T, func()) {
	if h == nil {
		values := make(chan T)
		var once sync.Once
		return values, func() { once.Do(func() { close(values) }) }
	}
	if h.local == nil {
		return NewLocalHub(h.config).Subscribe(key)
	}
	return h.local.Subscribe(key)
}

// PublishLocalOnce fans out a value while suppressing Redis echoes by ID.
func (h *RedisHub[T]) PublishLocalOnce(value T) {
	if h == nil || h.local == nil {
		return
	}
	value = normalize(h.config, value)
	id := h.config.ID(value)
	if id == "" {
		h.local.Publish(value)
		return
	}
	now := time.Now().UTC()
	h.mu.Lock()
	for seenID, expiresAt := range h.seen {
		if !expiresAt.After(now) {
			delete(h.seen, seenID)
		}
	}
	if _, ok := h.seen[id]; ok {
		h.mu.Unlock()
		return
	}
	h.seen[id] = now.Add(eventSeenTTL)
	h.mu.Unlock()
	h.local.Publish(value)
}

// Close stops Redis subscription and reconnect work.
func (h *RedisHub[T]) Close() error {
	if h == nil {
		return nil
	}
	h.cancel()
	<-h.done
	return nil
}

// subscribe reconnects Redis Pub/Sub and copies remote messages into local fanout.
func (h *RedisHub[T]) subscribe() {
	defer close(h.done)
	for h.ctx.Err() == nil {
		pubsub := h.client.Subscribe(h.ctx, h.config.Channel)
		if _, err := pubsub.Receive(h.ctx); err != nil {
			_ = pubsub.Close()
			if h.ctx.Err() != nil {
				return
			}
			h.log("subscribe_failed", err)
			select {
			case <-time.After(redisSubscribeRetryDelay):
			case <-h.ctx.Done():
				return
			}
			continue
		}
		for message := range pubsub.Channel() {
			var value T
			if err := json.Unmarshal([]byte(message.Payload), &value); err != nil {
				h.log("unmarshal_failed", err)
				continue
			}
			value = normalize(h.config, value)
			if h.config.Key(value) != "" {
				h.PublishLocalOnce(value)
			}
		}
		_ = pubsub.Close()
	}
}

// log emits a domain-prefixed Redis fanout diagnostic.
func (h *RedisHub[T]) log(action string, err error) {
	prefix := h.config.LogPrefix
	if prefix == "" {
		prefix = "realtime"
	}
	log.Printf("%s_%s error=%v", prefix, action, err)
}

// normalize applies an optional domain-specific value normalizer.
func normalize[T any](config Config[T], value T) T {
	if config.Normalize != nil {
		return config.Normalize(value)
	}
	return value
}

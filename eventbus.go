// Package eventbus provides a lightweight in-memory publish/subscribe system.
//
// Key characteristics:
//   - Non-blocking publish: slow subscribers don't block publishers
//   - At-most-once delivery: suitable for reconciliation-driven architectures
//   - Channel ownership: subscribers create and close their channels
//   - Thread-safe: concurrent publish/subscribe operations are safe
//   - Zero dependencies: only the Go standard library
package eventbus

import (
	"sync"
	"sync/atomic"
)

// Logger defines the logging interface for EventBus.
// Compatible with slog-style structured loggers.
type Logger interface {
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
}

// Event represents a typed event that can be published through the EventBus.
type Event interface {
	EventType() string
}

// subscriber holds a subscription channel and metadata.
type subscriber struct {
	ch chan Event
	id uint64
}

// EventBus provides an in-memory publish/subscribe system for decoupled
// communication between components.
//
// Non-blocking publish ensures that slow consumers never stall publishers.
// At-most-once delivery is intentional: this bus is designed for systems
// where missed events trigger reconciliation from authoritative state,
// making guaranteed delivery unnecessary overhead.
type EventBus struct {
	logger       Logger
	subscribers  map[string][]subscriber
	subIDCounter atomic.Uint64
	mu           sync.RWMutex
}

// New creates a new EventBus with the provided logger.
func New(logger Logger) *EventBus {
	return &EventBus{
		subscribers: make(map[string][]subscriber),
		logger:      logger,
	}
}

// Subscribe registers a channel to receive events of the specified type.
//
// The caller MUST provide a buffered channel and is responsible for closing it.
// Unbuffered channels cause a panic to prevent deadlocks.
//
// Returns an unsubscribe function that removes the subscription.
// The unsubscribe function is idempotent and safe to call multiple times.
//
// Example:
//
//	ch := make(chan eventbus.Event, 16)
//	unsubscribe := bus.Subscribe("order_created", ch)
//	defer unsubscribe()
//	defer close(ch)
//
//	for event := range ch {
//	    // handle event
//	}
func (eb *EventBus) Subscribe(eventType string, ch chan Event) func() {
	if cap(ch) == 0 {
		panic("eventbus: unbuffered channel provided to Subscribe — use a buffered channel to prevent deadlocks")
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	subID := eb.subIDCounter.Add(1)

	sub := subscriber{
		id: subID,
		ch: ch,
	}

	eb.subscribers[eventType] = append(eb.subscribers[eventType], sub)

	eb.logger.Debug("subscriber registered",
		"event_type", eventType,
		"subscriber_id", subID,
		"channel_capacity", cap(ch))

	return func() {
		eb.unsubscribe(eventType, subID)
	}
}

// unsubscribe removes a subscription by ID.
func (eb *EventBus) unsubscribe(eventType string, subID uint64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	subs := eb.subscribers[eventType]
	for i, sub := range subs {
		if sub.id == subID {
			eb.subscribers[eventType] = append(subs[:i], subs[i+1:]...)

			eb.logger.Debug("subscriber unregistered",
				"event_type", eventType,
				"subscriber_id", subID)
			return
		}
	}
}

// Publish sends an event to all subscribers of its type.
//
// Publishing is non-blocking: if a subscriber's channel is full,
// the event is dropped for that subscriber and a warning is logged.
// This prevents slow subscribers from blocking the entire system.
func (eb *EventBus) Publish(event Event) {
	eventType := event.EventType()

	eb.mu.RLock()
	subs := make([]subscriber, len(eb.subscribers[eventType]))
	copy(subs, eb.subscribers[eventType])
	eb.mu.RUnlock()

	if len(subs) == 0 {
		return
	}

	delivered := 0
	dropped := 0

	for _, sub := range subs {
		select {
		case sub.ch <- event:
			delivered++
		default:
			dropped++
			eb.logger.Warn("event dropped for slow subscriber",
				"event_type", eventType,
				"subscriber_id", sub.id,
				"channel_len", len(sub.ch),
				"channel_cap", cap(sub.ch))
		}
	}

	if delivered > 0 || dropped > 0 {
		eb.logger.Debug("event published",
			"event_type", eventType,
			"delivered", delivered,
			"dropped", dropped)
	}
}

// SubscriberCount returns the number of active subscribers for an event type.
func (eb *EventBus) SubscriberCount(eventType string) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers[eventType])
}

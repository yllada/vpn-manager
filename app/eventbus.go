// Package app provides a centralized event bus for VPN Manager.
// This implements a type-safe, thread-safe publish-subscribe pattern
// for decoupled communication between components.
//
// The event bus prevents tight coupling between UI and backend,
// enables clean testing, and provides a single source of truth
// for application state changes.
package app

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// EventType identifies the type of event.
type EventType string

// Event types for VPN operations
const (
	// Connection events
	EventConnectionStarting    EventType = "connection.starting"
	EventConnectionEstablished EventType = "connection.established"
	EventConnectionFailed      EventType = "connection.failed"
	EventConnectionLost        EventType = "connection.lost"
	EventConnectionClosed      EventType = "connection.closed"
	EventReconnecting          EventType = "connection.reconnecting"

	// Status events
	EventStatusChanged EventType = "status.changed"
	EventHealthChanged EventType = "health.changed"
	EventBytesUpdated  EventType = "bytes.updated"

	// Profile events
	EventProfileAdded   EventType = "profile.added"
	EventProfileUpdated EventType = "profile.updated"
	EventProfileDeleted EventType = "profile.deleted"
	EventProfileUsed    EventType = "profile.used"

	// Provider events
	EventProviderAvailable   EventType = "provider.available"
	EventProviderUnavailable EventType = "provider.unavailable"

	// Security events
	EventKillSwitchEnabled   EventType = "security.killswitch.enabled"
	EventKillSwitchDisabled  EventType = "security.killswitch.disabled"
	EventDNSProtectionOn     EventType = "security.dns.enabled"
	EventDNSProtectionOff    EventType = "security.dns.disabled"
	EventIPv6ProtectionOn    EventType = "security.ipv6.enabled"
	EventIPv6ProtectionOff   EventType = "security.ipv6.disabled"
	EventPotentialLeak       EventType = "security.leak.detected"

	// Authentication events
	EventAuthRequired EventType = "auth.required"
	EventAuthOTPRequired  EventType = "auth.otp.required"
	EventAuthFailed   EventType = "auth.failed"
	EventAuthSuccess  EventType = "auth.success"

	// Error events
	EventError   EventType = "error.occurred"
	EventWarning EventType = "warning.occurred"

	// System events
	EventShutdown EventType = "system.shutdown"
)

// Event represents an event in the system.
type Event struct {
	// Type identifies the event kind.
	Type EventType
	// Source identifies the component that generated the event.
	Source string
	// Timestamp is when the event was created.
	Timestamp time.Time
	// Data contains event-specific payload.
	Data interface{}
	// Context for cancellation and deadlines.
	Context context.Context
}

// NewEvent creates a new event with current timestamp.
func NewEvent(eventType EventType, source string, data interface{}) *Event {
	return &Event{
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
		Context:   context.Background(),
	}
}

// WithContext returns a copy of the event with the given context.
func (e *Event) WithContext(ctx context.Context) *Event {
	copy := *e
	copy.Context = ctx
	return &copy
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT DATA TYPES
// ═══════════════════════════════════════════════════════════════════════════

// ConnectionEventData contains data for connection events.
type ConnectionEventData struct {
	ProfileID    string
	ProfileName  string
	ProviderType VPNProviderType
	Status       ConnectionStatus
	IPAddress    string
	Error        error
	Attempt      int // For reconnection events
	MaxAttempts  int
}

// BytesEventData contains data for bandwidth updates.
type BytesEventData struct {
	ProfileID string
	BytesSent uint64
	BytesRecv uint64
	Duration  time.Duration
}

// HealthEventData contains data for health changes.
type HealthEventData struct {
	ProfileID string
	OldState  string
	NewState  string
	Latency   time.Duration
}

// SecurityEventData contains data for security events.
type SecurityEventData struct {
	Feature     string // e.g., "killswitch", "dns", "ipv6"
	Enabled     bool
	Interface   string
	DNSServers  []string
	LeakDetails string
}

// ErrorEventData contains data for error events.
type ErrorEventData struct {
	Code       ErrorCode
	Category   ErrorCategory
	Message    string
	Error      error
	Recoverable bool
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT HANDLER
// ═══════════════════════════════════════════════════════════════════════════

// EventHandler is a function that handles events.
type EventHandler func(*Event)

// TypedEventHandler handles events with type-safe data extraction.
type TypedEventHandler[T any] func(eventType EventType, data T)

// Subscription represents an active event subscription.
type Subscription struct {
	id         uint64
	eventType  EventType
	handler    EventHandler
	filter     func(*Event) bool
	once       bool
	bus        *EventBus
}

// Unsubscribe removes this subscription from the event bus.
func (s *Subscription) Unsubscribe() {
	if s.bus != nil {
		s.bus.unsubscribe(s.id)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT BUS
// ═══════════════════════════════════════════════════════════════════════════

// EventBus is the central hub for event distribution.
type EventBus struct {
	mu            sync.RWMutex
	subscriptions map[EventType][]*Subscription
	allHandlers   []*Subscription // Handlers for all events
	nextID        uint64
	
	// Async settings
	asyncWorkers  int
	eventQueue    chan *Event
	wg            sync.WaitGroup
	closed        atomic.Bool
	
	// Metrics
	published     uint64
	delivered     uint64
	dropped       uint64
}

// EventBusConfig configures the event bus behavior.
type EventBusConfig struct {
	// AsyncWorkers is the number of goroutines for async event delivery.
	// Set to 0 for synchronous delivery.
	AsyncWorkers int
	// QueueSize is the buffer size for async event queue.
	QueueSize int
}

// DefaultEventBusConfig returns sensible defaults.
func DefaultEventBusConfig() EventBusConfig {
	return EventBusConfig{
		AsyncWorkers: 4,
		QueueSize:    1000,
	}
}

// Global event bus instance
var (
	globalBus     *EventBus
	globalBusOnce sync.Once
)

// GetEventBus returns the global event bus instance.
func GetEventBus() *EventBus {
	globalBusOnce.Do(func() {
		globalBus = NewEventBus(DefaultEventBusConfig())
	})
	return globalBus
}

// NewEventBus creates a new event bus with the given configuration.
func NewEventBus(config EventBusConfig) *EventBus {
	bus := &EventBus{
		subscriptions: make(map[EventType][]*Subscription),
		allHandlers:   make([]*Subscription, 0),
		asyncWorkers:  config.AsyncWorkers,
	}

	if config.AsyncWorkers > 0 {
		bus.eventQueue = make(chan *Event, config.QueueSize)
		bus.startWorkers()
	}

	return bus
}

// startWorkers starts async event delivery workers.
func (bus *EventBus) startWorkers() {
	for i := 0; i < bus.asyncWorkers; i++ {
		bus.wg.Add(1)
		go bus.worker()
	}
}

func (bus *EventBus) worker() {
	defer bus.wg.Done()

	for event := range bus.eventQueue {
		bus.deliverEvent(event)
	}
}

// Subscribe registers a handler for a specific event type.
func (bus *EventBus) Subscribe(eventType EventType, handler EventHandler) *Subscription {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	sub := &Subscription{
		id:        atomic.AddUint64(&bus.nextID, 1),
		eventType: eventType,
		handler:   handler,
		bus:       bus,
	}

	bus.subscriptions[eventType] = append(bus.subscriptions[eventType], sub)
	return sub
}

// SubscribeAll registers a handler for all event types.
func (bus *EventBus) SubscribeAll(handler EventHandler) *Subscription {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	sub := &Subscription{
		id:        atomic.AddUint64(&bus.nextID, 1),
		eventType: "*",
		handler:   handler,
		bus:       bus,
	}

	bus.allHandlers = append(bus.allHandlers, sub)
	return sub
}

// SubscribeOnce registers a handler that fires only once.
func (bus *EventBus) SubscribeOnce(eventType EventType, handler EventHandler) *Subscription {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	sub := &Subscription{
		id:        atomic.AddUint64(&bus.nextID, 1),
		eventType: eventType,
		handler:   handler,
		once:      true,
		bus:       bus,
	}

	bus.subscriptions[eventType] = append(bus.subscriptions[eventType], sub)
	return sub
}

// SubscribeWithFilter registers a handler with custom filtering.
func (bus *EventBus) SubscribeWithFilter(eventType EventType, handler EventHandler, filter func(*Event) bool) *Subscription {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	sub := &Subscription{
		id:        atomic.AddUint64(&bus.nextID, 1),
		eventType: eventType,
		handler:   handler,
		filter:    filter,
		bus:       bus,
	}

	bus.subscriptions[eventType] = append(bus.subscriptions[eventType], sub)
	return sub
}

// unsubscribe removes a subscription by ID.
func (bus *EventBus) unsubscribe(id uint64) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	// Remove from specific event subscriptions
	for eventType, subs := range bus.subscriptions {
		newSubs := make([]*Subscription, 0, len(subs))
		for _, sub := range subs {
			if sub.id != id {
				newSubs = append(newSubs, sub)
			}
		}
		bus.subscriptions[eventType] = newSubs
	}

	// Remove from all handlers
	newAll := make([]*Subscription, 0, len(bus.allHandlers))
	for _, sub := range bus.allHandlers {
		if sub.id != id {
			newAll = append(newAll, sub)
		}
	}
	bus.allHandlers = newAll
}

// Publish sends an event to all subscribers.
func (bus *EventBus) Publish(event *Event) {
	if bus.closed.Load() {
		return
	}

	atomic.AddUint64(&bus.published, 1)

	if bus.asyncWorkers > 0 {
		// Async delivery
		select {
		case bus.eventQueue <- event:
		default:
			// Queue full, drop event
			atomic.AddUint64(&bus.dropped, 1)
			LogWarn("EventBus: queue full, dropping event: %s", event.Type)
		}
	} else {
		// Sync delivery
		bus.deliverEvent(event)
	}
}

// PublishSync ensures synchronous delivery regardless of config.
func (bus *EventBus) PublishSync(event *Event) {
	atomic.AddUint64(&bus.published, 1)
	bus.deliverEvent(event)
}

// deliverEvent sends the event to matching handlers.
func (bus *EventBus) deliverEvent(event *Event) {
	bus.mu.RLock()
	
	// Get handlers for this event type
	handlers := append([]*Subscription{}, bus.subscriptions[event.Type]...)
	allHandlers := append([]*Subscription{}, bus.allHandlers...)
	
	bus.mu.RUnlock()

	var toRemove []uint64

	// Deliver to specific handlers
	for _, sub := range handlers {
		if sub.filter != nil && !sub.filter(event) {
			continue
		}

		bus.safeDeliver(sub.handler, event)
		atomic.AddUint64(&bus.delivered, 1)

		if sub.once {
			toRemove = append(toRemove, sub.id)
		}
	}

	// Deliver to all-event handlers
	for _, sub := range allHandlers {
		if sub.filter != nil && !sub.filter(event) {
			continue
		}

		bus.safeDeliver(sub.handler, event)
		atomic.AddUint64(&bus.delivered, 1)
	}

	// Remove once-handlers
	for _, id := range toRemove {
		bus.unsubscribe(id)
	}
}

// safeDeliver calls handler with panic recovery.
func (bus *EventBus) safeDeliver(handler EventHandler, event *Event) {
	defer func() {
		if r := recover(); r != nil {
			LogError("EventBus: handler panic for event %s: %v", event.Type, r)
		}
	}()

	handler(event)
}

// Close shuts down the event bus gracefully.
func (bus *EventBus) Close() {
	if bus.closed.Swap(true) {
		return // Already closed
	}

	if bus.eventQueue != nil {
		close(bus.eventQueue)
		bus.wg.Wait()
	}
}

// Stats returns event bus statistics.
func (bus *EventBus) Stats() (published, delivered, dropped uint64) {
	return atomic.LoadUint64(&bus.published),
		atomic.LoadUint64(&bus.delivered),
		atomic.LoadUint64(&bus.dropped)
}

// ═══════════════════════════════════════════════════════════════════════════
// CONVENIENCE FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

// Emit is a shorthand for publishing an event through the global bus.
func Emit(eventType EventType, source string, data interface{}) {
	event := NewEvent(eventType, source, data)
	GetEventBus().Publish(event)
}

// On is a shorthand for subscribing to events on the global bus.
func On(eventType EventType, handler EventHandler) *Subscription {
	return GetEventBus().Subscribe(eventType, handler)
}

// Once is a shorthand for one-time subscription on the global bus.
func Once(eventType EventType, handler EventHandler) *Subscription {
	return GetEventBus().SubscribeOnce(eventType, handler)
}

// SubscribeTyped creates a type-safe subscription.
// It extracts the data field and casts it to the expected type.
func SubscribeTyped[T any](bus *EventBus, eventType EventType, handler TypedEventHandler[T]) *Subscription {
	return bus.Subscribe(eventType, func(event *Event) {
		if data, ok := event.Data.(T); ok {
			handler(event.Type, data)
		} else {
			// Try pointer type
			if data, ok := event.Data.(*T); ok && data != nil {
				handler(event.Type, *data)
			} else {
				LogWarn("EventBus: type mismatch for event %s, expected %s got %s",
					event.Type, reflect.TypeOf((*T)(nil)).Elem(), reflect.TypeOf(event.Data))
			}
		}
	})
}

// WaitForEvent waits for a specific event type with timeout.
func WaitForEvent(ctx context.Context, eventType EventType) (*Event, error) {
	ch := make(chan *Event, 1)

	sub := GetEventBus().SubscribeOnce(eventType, func(e *Event) {
		select {
		case ch <- e:
		default:
		}
	})
	defer sub.Unsubscribe()

	select {
	case event := <-ch:
		return event, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

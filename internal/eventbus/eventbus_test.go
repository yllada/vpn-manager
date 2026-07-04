package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_Subscribe(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0}) // Sync mode

	received := false
	bus.Subscribe(EventConnectionEstablished, func(e *Event) {
		received = true
	})

	event := NewEvent(EventConnectionEstablished, "test", nil)
	bus.Publish(event)

	if !received {
		t.Error("Event handler was not called")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var count int32
	sub := bus.Subscribe(EventError, func(e *Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish(NewEvent(EventError, "test", nil))
	sub.Unsubscribe()
	bus.Publish(NewEvent(EventError, "test", nil))

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("Expected 1 call before unsubscribe, got %d", count)
	}
}

func TestEventBus_AsyncDelivery(t *testing.T) {
	bus := NewEventBus(EventBusConfig{
		AsyncWorkers: 2,
		QueueSize:    100,
	})
	defer bus.Close()

	var count int32
	var wg sync.WaitGroup
	wg.Add(10)

	bus.Subscribe(EventStatusChanged, func(e *Event) {
		atomic.AddInt32(&count, 1)
		wg.Done()
	})

	for i := 0; i < 10; i++ {
		bus.Publish(NewEvent(EventStatusChanged, "test", i))
	}

	// Wait for all events to be delivered
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Error("Timeout waiting for async delivery")
	}

	if atomic.LoadInt32(&count) != 10 {
		t.Errorf("Expected 10 events, got %d", count)
	}
}

func TestEventBus_Stats(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	bus.Subscribe(EventStatusChanged, func(e *Event) {})

	for i := 0; i < 5; i++ {
		bus.Publish(NewEvent(EventStatusChanged, "test", nil))
	}

	published, delivered, dropped := bus.Stats()

	if published != 5 {
		t.Errorf("Expected 5 published, got %d", published)
	}
	if delivered != 5 {
		t.Errorf("Expected 5 delivered, got %d", delivered)
	}
	if dropped != 0 {
		t.Errorf("Expected 0 dropped, got %d", dropped)
	}
}

func TestEventBus_PanicRecovery(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var count int32
	bus.Subscribe(EventError, func(e *Event) {
		atomic.AddInt32(&count, 1)
		panic("test panic")
	})

	// Should not panic
	bus.Publish(NewEvent(EventError, "test", nil))
	bus.Publish(NewEvent(EventError, "test", nil))

	// Handler should still be called each time (panic recovered)
	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("Expected 2 calls despite panics, got %d", count)
	}
}

func TestEventBus_EventData(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var receivedData ConnectionEventData
	bus.Subscribe(EventConnectionEstablished, func(e *Event) {
		if data, ok := e.Data.(ConnectionEventData); ok {
			receivedData = data
		}
	})

	data := ConnectionEventData{
		ProfileID:   "test-profile",
		ProfileName: "Test VPN",
		IPAddress:   "10.0.0.1",
	}

	bus.Publish(NewEvent(EventConnectionEstablished, "vpn", data))

	if receivedData.ProfileID != "test-profile" {
		t.Error("Event data not received correctly")
	}
	if receivedData.IPAddress != "10.0.0.1" {
		t.Error("Event data IP not received correctly")
	}
}

func TestEvent_WithContext(t *testing.T) {
	event := NewEvent(EventStatusChanged, "test", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventWithCtx := event.WithContext(ctx)

	if eventWithCtx.Context != ctx {
		t.Error("Context not set correctly")
	}

	// Original event should be unchanged
	if event.Context == ctx {
		t.Error("Original event should not be modified")
	}
}

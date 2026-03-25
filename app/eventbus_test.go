package app

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

func TestEventBus_SubscribeAll(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var count int32
	bus.SubscribeAll(func(e *Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish(NewEvent(EventConnectionStarting, "test", nil))
	bus.Publish(NewEvent(EventConnectionEstablished, "test", nil))
	bus.Publish(NewEvent(EventConnectionClosed, "test", nil))

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

func TestEventBus_SubscribeOnce(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var count int32
	bus.SubscribeOnce(EventStatusChanged, func(e *Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish(NewEvent(EventStatusChanged, "test", nil))
	bus.Publish(NewEvent(EventStatusChanged, "test", nil))
	bus.Publish(NewEvent(EventStatusChanged, "test", nil))

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("Expected 1 call (once), got %d", count)
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

func TestEventBus_SubscribeWithFilter(t *testing.T) {
	bus := NewEventBus(EventBusConfig{AsyncWorkers: 0})

	var count int32
	bus.SubscribeWithFilter(EventConnectionEstablished, func(e *Event) {
		atomic.AddInt32(&count, 1)
	}, func(e *Event) bool {
		// Only accept events from "vpn" source
		return e.Source == "vpn"
	})

	bus.Publish(NewEvent(EventConnectionEstablished, "vpn", nil))
	bus.Publish(NewEvent(EventConnectionEstablished, "other", nil))
	bus.Publish(NewEvent(EventConnectionEstablished, "vpn", nil))

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("Expected 2 filtered events, got %d", count)
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

func TestEventBus_PublishSync(t *testing.T) {
	bus := NewEventBus(EventBusConfig{
		AsyncWorkers: 2,
		QueueSize:    100,
	})
	defer bus.Close()

	received := false
	bus.Subscribe(EventShutdown, func(e *Event) {
		received = true
	})

	// This should be synchronous
	bus.PublishSync(NewEvent(EventShutdown, "test", nil))

	if !received {
		t.Error("Sync publish should deliver immediately")
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

func TestWaitForEvent(t *testing.T) {
	// Reset global bus for this test
	oldBus := globalBus
	globalBus = NewEventBus(EventBusConfig{AsyncWorkers: 0})
	defer func() { globalBus = oldBus }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Publish after a delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		Emit(EventConnectionEstablished, "test", "data")
	}()

	event, err := WaitForEvent(ctx, EventConnectionEstablished)
	if err != nil {
		t.Errorf("WaitForEvent failed: %v", err)
	}
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Data != "data" {
		t.Error("Event data mismatch")
	}
}

func TestWaitForEvent_Timeout(t *testing.T) {
	// Reset global bus for this test
	oldBus := globalBus
	globalBus = NewEventBus(EventBusConfig{AsyncWorkers: 0})
	defer func() { globalBus = oldBus }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := WaitForEvent(ctx, EventConnectionEstablished)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

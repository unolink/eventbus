package eventbus

import (
	"sync"
	"testing"
	"time"
)

type mockLogger struct {
	warns  []string
	debugs []string
	mu     sync.Mutex
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warns = append(m.warns, msg)
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.debugs = append(m.debugs, msg)
}

func (m *mockLogger) warnCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.warns)
}

type testEvent struct {
	eventType string
	payload   string
}

func (e testEvent) EventType() string {
	return e.eventType
}

func TestSubscribeAndPublish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event     Event
		name      string
		eventType string
		wantRecv  bool
	}{
		{
			name:      "matching event type",
			eventType: "order_created",
			event:     testEvent{eventType: "order_created", payload: "data"},
			wantRecv:  true,
		},
		{
			name:      "non-matching event type",
			eventType: "order_created",
			event:     testEvent{eventType: "order_deleted", payload: "data"},
			wantRecv:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bus := New(&mockLogger{})

			ch := make(chan Event, 1)
			unsubscribe := bus.Subscribe(tt.eventType, ch)
			defer unsubscribe()
			defer close(ch)

			bus.Publish(tt.event)

			time.Sleep(10 * time.Millisecond)

			select {
			case recv := <-ch:
				if !tt.wantRecv {
					t.Errorf("received event when none expected: %v", recv)
				}
			default:
				if tt.wantRecv {
					t.Error("expected to receive event but got none")
				}
			}
		})
	}
}

func TestMultipleSubscribers(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	const numSubscribers = 5
	channels := make([]chan Event, numSubscribers)
	var unsubscribes []func()

	for i := 0; i < numSubscribers; i++ {
		ch := make(chan Event, 1)
		channels[i] = ch
		unsubscribes = append(unsubscribes, bus.Subscribe("broadcast", ch))
	}

	bus.Publish(testEvent{eventType: "broadcast", payload: "message"})

	time.Sleep(10 * time.Millisecond)

	for i, ch := range channels {
		select {
		case recv := <-ch:
			if recv.EventType() != "broadcast" {
				t.Errorf("subscriber %d: got event type %q, want %q", i, recv.EventType(), "broadcast")
			}
		default:
			t.Errorf("subscriber %d: did not receive event", i)
		}
	}

	for i, unsub := range unsubscribes {
		unsub()
		close(channels[i])
	}
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	ch := make(chan Event, 1)
	unsubscribe := bus.Subscribe("test", ch)

	if count := bus.SubscriberCount("test"); count != 1 {
		t.Errorf("got subscriber count %d, want 1", count)
	}

	unsubscribe()

	if count := bus.SubscriberCount("test"); count != 0 {
		t.Errorf("after unsubscribe: got subscriber count %d, want 0", count)
	}

	bus.Publish(testEvent{eventType: "test", payload: "data"})
	time.Sleep(10 * time.Millisecond)

	select {
	case recv := <-ch:
		t.Errorf("received event after unsubscribe: %v", recv)
	default:
	}

	close(ch)
}

func TestNonBlockingPublish(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	bus := New(logger)

	ch := make(chan Event, 1)
	unsubscribe := bus.Subscribe("test", ch)
	defer unsubscribe()
	defer close(ch)

	bus.Publish(testEvent{eventType: "test", payload: "first"})
	time.Sleep(10 * time.Millisecond)

	bus.Publish(testEvent{eventType: "test", payload: "second"})
	time.Sleep(10 * time.Millisecond)

	if logger.warnCount() == 0 {
		t.Error("expected warning about dropped event, got none")
	}

	select {
	case recv := <-ch:
		if me := recv.(testEvent); me.payload != "first" {
			t.Errorf("got payload %q, want %q", me.payload, "first")
		}
	default:
		t.Fatal("expected first event in channel")
	}

	select {
	case recv := <-ch:
		t.Errorf("got unexpected second event: %v", recv)
	default:
	}
}

func TestEventTypeFiltering(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	ch1 := make(chan Event, 5)
	ch2 := make(chan Event, 5)

	unsub1 := bus.Subscribe("type_a", ch1)
	unsub2 := bus.Subscribe("type_b", ch2)
	defer unsub1()
	defer unsub2()
	defer close(ch1)
	defer close(ch2)

	bus.Publish(testEvent{eventType: "type_a", payload: "a1"})
	bus.Publish(testEvent{eventType: "type_b", payload: "b1"})
	bus.Publish(testEvent{eventType: "type_a", payload: "a2"})

	time.Sleep(10 * time.Millisecond)

	var ch1Events []testEvent
	for len(ch1) > 0 {
		e := <-ch1
		ch1Events = append(ch1Events, e.(testEvent))
	}

	if len(ch1Events) != 2 {
		t.Errorf("ch1: got %d events, want 2", len(ch1Events))
	}

	var ch2Events []testEvent
	for len(ch2) > 0 {
		e := <-ch2
		ch2Events = append(ch2Events, e.(testEvent))
	}

	if len(ch2Events) != 1 {
		t.Errorf("ch2: got %d events, want 1", len(ch2Events))
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	const numGoroutines = 10
	const eventsPerGoroutine = 20

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				bus.Publish(testEvent{eventType: "concurrent", payload: "data"})
			}
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := make(chan Event, 100)
			unsub := bus.Subscribe("concurrent", ch)
			defer unsub()

			timeout := time.After(100 * time.Millisecond)
			for {
				select {
				case <-ch:
				case <-timeout:
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestUnbufferedChannelPanic(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unbuffered channel, got none")
		}
	}()

	ch := make(chan Event)
	bus.Subscribe("test", ch)
}

func TestIdempotentUnsubscribe(t *testing.T) {
	t.Parallel()

	bus := New(&mockLogger{})

	ch := make(chan Event, 1)
	unsubscribe := bus.Subscribe("test", ch)
	defer close(ch)

	unsubscribe()
	unsubscribe()
	unsubscribe()

	if count := bus.SubscriberCount("test"); count != 0 {
		t.Errorf("got subscriber count %d, want 0", count)
	}
}

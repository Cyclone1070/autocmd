package workflow

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestBroker_Ordering(t *testing.T) {
	out := make(chan domain.Event, 100)
	broker := NewEventBroker(out)

	count := 50
	for i := 0; i < count; i++ {
		broker.Send(domain.TextEvent{Text: string(rune(i))})
	}

	for i := 0; i < count; i++ {
		ev := <-out
		assert.Equal(t, string(rune(i)), ev.(domain.TextEvent).Text)
	}
}

func TestBroker_NonBlocking(t *testing.T) {
	// Unbuffered output channel
	out := make(chan domain.Event)
	broker := NewEventBroker(out)

	// Send without a consumer. Should not hang.
	done := make(chan bool)
	go func() {
		broker.Send(domain.TextEvent{Text: "heavy"})
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Send() blocked")
	}
}

func TestBroker_CloseFlushes(t *testing.T) {
	out := make(chan domain.Event, 10)
	broker := NewEventBroker(out)

	broker.Send(domain.TextEvent{Text: "1"})
	broker.Send(domain.TextEvent{Text: "2"})

	// Close should only return AFTER "1" and "2" are pushed to 'out'
	broker.Close()

	assert.Equal(t, 2, len(out))
	assert.Equal(t, "1", (<-out).(domain.TextEvent).Text)
	assert.Equal(t, "2", (<-out).(domain.TextEvent).Text)
}

func TestBroker_CloseDoesNotCloseOutput(t *testing.T) {
	out := make(chan domain.Event, 10)
	broker := NewEventBroker(out)

	broker.Close()

	// Sending to 'out' directly should NOT panic
	assert.NotPanics(t, func() {
		out <- domain.TextEvent{Text: "still alive"}
	})
}

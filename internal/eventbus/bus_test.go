package eventbus

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestEventBus_Ownership(t *testing.T) {
	// RED: NewEventBus takes no arguments
	bus := New()
	defer bus.Close()

	// RED: Provides unidirectional accessors
	assert.NotNil(t, bus.UIUpdates())
	assert.NotNil(t, bus.WorkflowActions())
}

func TestEventBus_BiDirectional_Centralized(t *testing.T) {
	bus := New()
	defer bus.Close()

	uiOut := bus.UIUpdates()
	wfOut := bus.WorkflowActions()

	// 1. Workflow -> UI (UIUpdateSender)
	bus.SendUIUpdate(domain.TextEvent{Text: "from-workflow"})
	select {
	case ev := <-uiOut:
		assert.Equal(t, "from-workflow", ev.(domain.TextEvent).Text)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("UI update not received")
	}

	// 2. UI -> Workflow (UIActionSender)
	bus.SendAction(domain.StopAction{})
	select {
	case act := <-wfOut:
		assert.IsType(t, domain.StopAction{}, act)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Action not received")
	}
}

func TestEventBus_Shutdown_NoHang(t *testing.T) {
	bus := New()
	uiOut := bus.UIUpdates()

	// Fill more than the channel buffer
	count := 200
	go func() {
		for range count {
			bus.SendUIUpdate(domain.TextEvent{Text: "msg"})
		}
	}()

	// Close should return quickly and not hang, even if consumer is slow
	done := make(chan struct{})
	go func() {
		bus.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success: Close() did not deadlock
	case <-time.After(1 * time.Second):
		t.Fatal("Close() hung during shutdown")
	}

	// Consume what's available
	received := 0
	for range uiOut {
		received++
	}
	// We don't assert 'count == received' because we explicitly want to allow discarding
	// to prevent deadlocks.
}

func TestEventBus_Concurrent_Safe(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Multiple goroutines sending updates and actions
	for range 10 {
		go func() {
			bus.SendUIUpdate(domain.ThinkingEvent{})
			bus.SendAction(domain.StopAction{})
		}()
	}
	// No panic = success for this basic concurrency test
}

func TestEventBus_CloseDeadlock_Reproduction(t *testing.T) {
	// Root cause: Close() waits for goroutines that block on Send if no one is reading.
	bus := New()

	// The internal channels have a buffer of 100.
	// We send 101 messages.
	// 100 go into the output channel buffer.
	// 1 stays in the internal 'queue' slice.
	for range 101 {
		bus.SendUIUpdate(domain.TextEvent{Text: "deadlock-me"})
	}

	// We do NOT read from the UIUpdates() channel.

	done := make(chan struct{})
	go func() {
		// This will call wg.Wait()
		// runUIUpdates will try to push the 101st message to uiOut
		// uiOut is full (100 msgs) and no one is reading.
		// runUIUpdates blocks on 'b.uiOut <- qe'
		// wg.Wait() blocks forever.
		bus.Close()
		close(done)
	}()

	select {
	case <-done:
		// If we reach here, the deadlock is fixed (or didn't happen)
	case <-time.After(2 * time.Second):
		t.Fatal("FATAL: Deadlock detected in Close(). The call hung for 2 seconds.")
	}
}

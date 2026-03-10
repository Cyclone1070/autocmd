package workflow

import (
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestEventBus_Ownership(t *testing.T) {
	// RED: NewEventBus takes no arguments
	bus := NewEventBus()
	defer bus.Close()

	// RED: Provides unidirectional accessors
	assert.NotNil(t, bus.UIUpdates())
	assert.NotNil(t, bus.WorkflowActions())
}

func TestEventBus_BiDirectional_Centralized(t *testing.T) {
	bus := NewEventBus()
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

func TestEventBus_Drainage_Comprehensive(t *testing.T) {
	bus := NewEventBus()
	uiOut := bus.UIUpdates()

	count := 100
	for i := 0; i < count; i++ {
		bus.SendUIUpdate(domain.TextEvent{Text: "msg"})
	}

	// Close immediately
	bus.Close()

	received := 0
	for range uiOut {
		received++
	}
	assert.Equal(t, count, received, "All UI updates must be received after Close()")

	// Workflow actions should also drain
	bus2 := NewEventBus()
	wfOut := bus2.WorkflowActions()
	for i := 0; i < count; i++ {
		bus2.SendAction(domain.StopAction{})
	}
	bus2.Close()

	receivedWf := 0
	for range wfOut {
		receivedWf++
	}
	assert.Equal(t, count, receivedWf, "All Actions must be received after Close()")
}

func TestEventBus_Concurrent_Safe(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	// Multiple goroutines sending updates and actions
	for i := 0; i < 10; i++ {
		go func() {
			bus.SendUIUpdate(domain.ThinkingEvent{})
			bus.SendAction(domain.StopAction{})
		}()
	}
	// No panic = success for this basic concurrency test
}

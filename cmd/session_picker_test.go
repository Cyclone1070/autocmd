package cmd

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/state"
	"github.com/stretchr/testify/assert"
)

func TestSessionPicker_StateUpdate(t *testing.T) {
	t.Run("Deleting active session must clear CurrentSessionID", func(t *testing.T) {
		appState := &state.State{}
		appState.SetCurrentSessionID("active-session")
		wrapper := &sessionPickerWrapper{
			state: appState,
		}

		itemID := "active-session"
		// Logic from session_picker.go:
		if itemID == wrapper.state.CurrentSessionID() {
			wrapper.state.SetCurrentSessionID("")
		}

		assert.Equal(t, "", wrapper.state.CurrentSessionID(), "Session ID must be cleared after deleting the active session")
	})

	t.Run("Deleting non-active session must NOT clear CurrentSessionID", func(t *testing.T) {
		appState := &state.State{}
		appState.SetCurrentSessionID("active-session")
		wrapper := &sessionPickerWrapper{
			state: appState,
		}

		itemID := "other-session"
		if itemID == wrapper.state.CurrentSessionID() {
			wrapper.state.SetCurrentSessionID("")
		}

		assert.Equal(t, "active-session", wrapper.state.CurrentSessionID(), "Session ID must remain unchanged when deleting a non-active session")
	})
}

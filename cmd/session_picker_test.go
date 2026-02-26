package cmd

import (
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionPicker_RenameBug(t *testing.T) {
	// Setup a temporary session store
	tmpDir, err := os.MkdirTemp("", "iav-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := config.DefaultConfig()
	cfg.Session.StorageDir = tmpDir
	fileSystem := fs.NewOSFileSystem(cfg)
	store := session.NewStore(cfg, fileSystem)

	// Create a session
	sess, err := store.Create()
	require.NoError(t, err)
	sess.Name = "original name"
	err = store.Save(sess)
	require.NoError(t, err)

	summaries, err := store.List()
	require.NoError(t, err)

	ti := textinput.New()
	wrapper := &sessionPickerWrapper{
		cfg:       cfg,
		store:     store,
		textInput: ti,
	}

	pickerCfg := ui.Config{
		Title: "SESSIONS",
		Items: wrapper.mapSessionsToItems(summaries),
		Actions: []ui.Action{
			{
				Key:   "r",
				Label: "rename",
				Fn: func(item ui.Item) tea.Cmd {
					wrapper.renaming = true
					wrapper.renameItemID = item.ID
					wrapper.textInput.SetValue(item.Label)
					wrapper.textInput.Focus()
					return textinput.Blink
				},
			},
		},
	}
	wrapper.picker = ui.NewPicker(pickerCfg)

	// 1. Press 'r' to start renaming
	_, _ = wrapper.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	assert.True(t, wrapper.renaming)
	assert.Equal(t, "original name", wrapper.textInput.Value())

	// 2. Type new name
	wrapper.textInput.SetValue("new name")

	// 3. Press Enter to confirm
	_, _ = wrapper.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, wrapper.renaming)

	// Verify if it's renamed
	updatedSess, err := store.Get(sess.ID)
	require.NoError(t, err)
	// THIS IS EXPECTED TO FAIL IF THE BUG IS PRESENT
	assert.Equal(t, "new name", updatedSess.Name, "Session should have been renamed")
}

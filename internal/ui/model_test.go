package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// Helper to create model for testing
// MockCursorDetector implements CursorDetector for testing
type mockCursorDetector struct {
	row int
	err error
}

func (m mockCursorDetector) GetCursorRow() (int, error) {
	return m.row, m.err
}

func newTestModel(t *testing.T) *model {
	t.Helper()
	cfg := config.DefaultConfig()
	// Inject mock detector returning row 1
	cd := mockCursorDetector{row: 1}
	m, err := newModel(cfg, cd)
	if err != nil {
		t.Fatalf("Failed to create test model: %v", err)
	}
	// For testing, mock terminal size to something standard
	m.width = 80
	m.termHeight = 24
	return m
}

func TestModel_Init(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestModel_ThinkingEvent(t *testing.T) {
	m := newTestModel(t)

	// Send ThinkingEvent
	updatedM, cmd := m.Update(msg{Event: domain.ThinkingEvent{}})

	// Check state
	newM := updatedM.(*model)
	assert.True(t, newM.thinking)
	assert.NotNil(t, cmd) // Should trigger tick

	// Check View
	view := newM.View()
	assert.Contains(t, view, "Thinking")

	// Reset
	m = newTestModel(t)
	m.Update(msg{Event: domain.TextEvent{Text: "Done"}})
	newM = m
	assert.False(t, newM.thinking)
}

func TestModel_CtrlC(t *testing.T) {
	m := newTestModel(t)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
	assert.Equal(t, stateCancelled, newM.(*model).runState)
}

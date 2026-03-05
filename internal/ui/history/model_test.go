package history

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestModel_WidthCapping(t *testing.T) {
	cfg := config.UIConfig{
		ChatWindowWidth: 80,
	}
	messages := []domain.Message{}

	// Case 1: Terminal is wider than config -> should cap to config
	m := NewModel(messages, cfg, 200, 40)
	assert.Equal(t, 80, m.width)

	// Case 2: Terminal is narrower than config -> should cap to terminal
	m = NewModel(messages, cfg, 40, 40)
	assert.Equal(t, 40, m.width)

	// Case 3: Resize to larger than config -> should stay capped at config
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = tm.(*Model)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 50, m.height)

	// Case 4: Resize to smaller than config -> should follow terminal
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 50})
	m = tm.(*Model)
	assert.Equal(t, 30, m.width)
}

func TestModel_EmptyMessages_NoPanic(t *testing.T) {
	cfg := config.DefaultConfig().UI
	messages := []domain.Message{}
	m := NewModel(messages, cfg, 80, 20)

	// Should not panic on resize
	assert.NotPanics(t, func() {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	})
	assert.True(t, m.reachedTop)
}

func TestModel_ResizeBehavior(t *testing.T) {
	cfg := config.UIConfig{ChatWindowWidth: 100}
	messages := []domain.Message{
		{Role: domain.RoleUser, Content: "hello"},
		{Role: domain.RoleAssistant, Content: "hi there"},
		{Role: domain.RoleUser, Content: "how are you?"},
		{Role: domain.RoleAssistant, Content: "i am good"},
	}

	t.Run("HeightOnlyResize_PreservesCache", func(t *testing.T) {
		m := NewModel(messages, cfg, 80, 20)
		// Trigger some rendering
		m.initializeContent()
		initialCacheSize := len(m.renderedMessages)
		assert.Greater(t, initialCacheSize, 0)

		// Resize height only
		tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
		m2 := tm.(*Model)

		assert.Equal(t, initialCacheSize, len(m2.renderedMessages), "Cache should be preserved when width stays the same")
	})

	t.Run("WidthResize_ClearsCache", func(t *testing.T) {
		m := NewModel(messages, cfg, 80, 20)
		m.initializeContent()

		// Resize width
		tm, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		m2 := tm.(*Model)

		assert.Equal(t, len(messages), len(m2.renderedMessages), "Cache should be reset and re-populated with newly rendered messages")
	})

	t.Run("ResizeSmaller_CanScrollToTop", func(t *testing.T) {
		// Start with 10 messages in a tall window - everything fits
		var manyMsg []domain.Message
		for i := 0; i < 10; i++ {
			manyMsg = append(manyMsg, domain.Message{Role: domain.RoleUser, Content: "msg"})
		}

		m := NewModel(manyMsg, cfg, 80, 100) // 100 lines height
		assert.True(t, m.reachedTop, "Should reach top when all messages fit")

		// Resize to very short window
		tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 2})
		m2 := tm.(*Model)

		// With height=2, limit=4 lines. 10 messages won't fit.
		assert.False(t, m2.reachedTop, "reachedTop should be reset so user can scroll up to load earlier messages")
		assert.Greater(t, m2.topIdx, 0)
	})
}

func TestIssue_History_ViewportGapAccumulation(t *testing.T) {
	// This test ensures that YOffset does not exceed mathematical bounds
	// due to the newline fusion over-counting bug.
	// Prepending messages must correctly account for the true line count of
	// the joined string rather than individual heights.

	cfg := config.DefaultConfig().UI
	var messages []domain.Message
	// Provide many messages to ensure we have content to prepend.
	for i := 0; i < 50; i++ {
		messages = append(messages, domain.Message{Role: domain.RoleUser, Content: "filler\n"})
	}

	// 1. Initialize with height 20.
	// initializeContent renders from messages backward up to limit=height*2 (40 lines).
	m := NewModel(messages, cfg, 80, 20)

	// 2. refreshViewport triggers if YOffset < height.
	// It must increment YOffset exactly by the number of mathematical lines added.
	m.refreshViewport()

	totalLines := m.viewport.TotalLineCount()
	maxAllowedY := totalLines - m.height
	if maxAllowedY < 0 {
		maxAllowedY = 0
	}

	assert.LessOrEqual(t, m.viewport.YOffset, maxAllowedY,
		"YOffset (%d) should be within content bounds [%d]. Failure indicates a blank gap at the bottom.",
		m.viewport.YOffset, maxAllowedY)
}

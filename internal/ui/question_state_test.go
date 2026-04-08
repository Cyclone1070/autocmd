package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func qDisplaySingle() domain.QuestionDisplay {
	return domain.NewQuestionDisplay([]domain.QuestionInfo{
		{
			Question: "Deploy where?",
			Options:  []string{"Staging", "Prod"},
			MultiSelect: false,
		},
	})
}

func qDisplayMultiTwoQuestions() domain.QuestionDisplay {
	return domain.NewQuestionDisplay([]domain.QuestionInfo{
		{
			Question: "Pick colors",
			Options:  []string{"Red", "Blue"},
			MultiSelect: true,
		},
		{
			Question: "Second?",
			Options:  []string{"Yes", "No"},
			MultiSelect: false,
		},
	})
}

func TestNewQuestionUIState_BuildsPerQuestionSlices(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	require.Len(t, s.Per, 1)
	assert.Len(t, s.Per[0].MultiSelected, 2)
	assert.Equal(t, 0, s.Active)
	assert.Equal(t, 0, s.Per[0].Cursor)
	assert.False(t, s.Submitted)
}

func TestHandleQuestionKey_LeftRightChangesActiveTab(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRight})
	assert.False(t, out.Done && !out.Cancelled)
	assert.Equal(t, 1, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 2, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 1, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 0, s.Active)
}

func TestHandleQuestionKey_UpDownMovesCursor(t *testing.T) {
	d := qDisplaySingle()
	// rows: 0–1 options only (no submit row)
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, s.Per[0].Cursor)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 0, s.Per[0].Cursor)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, s.Per[0].Cursor)
}

func TestHandleQuestionKey_SpaceMatchesEnterInMultiSelect(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{{
		Question: "Q", Options: []string{"A", "B"}, MultiSelect: true,
	}})
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, s.Per[0].MultiSelected[0])
	assert.Equal(t, 0, s.Per[0].Cursor)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, s.Per[0].MultiSelected[1])
}

func TestHandleQuestionKey_EnterOnAlreadySelectedSingleUnselects(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{Question: "Q1", Options: []string{"A", "B"}, MultiSelect: false},
		{Question: "Q2", Options: []string{"X"}, MultiSelect: false},
	})
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter}) // select A, advance
	require.Equal(t, 1, s.Active)
	require.Equal(t, 0, s.Per[0].SingleSelected)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
	require.Equal(t, 0, s.Active)
	s.Per[0].Cursor = 0
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.Equal(t, -1, s.Per[0].SingleSelected)
	assert.Equal(t, 0, s.Active)
	assert.False(t, s.Submitted)
}

func TestHandleQuestionKey_EnterSingleSelectAutoSubmits(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown}) // cursor on Prod
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, out.Done)
	assert.True(t, s.Submitted)
	assert.Equal(t, 1, s.Per[0].SingleSelected)
	assert.Equal(t, 1, s.Per[0].Cursor)

	require.Len(t, out.Answers, 1)
	assert.Equal(t, []string{"Prod"}, out.Answers[0])
}

func TestHandleQuestionKey_EnterMultiTogglesSelections(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{{
		Question: "Q", Options: []string{"A", "B"}, MultiSelect: true,
	}})
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, s.Per[0].MultiSelected[0])
	assert.True(t, s.Per[0].MultiSelected[1])
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.False(t, s.Submitted)
}

func TestHandleQuestionKey_EscCancels(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEsc})
	assert.True(t, out.Cancelled)
	assert.False(t, out.Done)
	assert.True(t, s.Submitted)
}

func TestHandleQuestionKey_QCancelsLikeEsc(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.True(t, out.Cancelled)
	assert.False(t, out.Done)
	assert.True(t, s.Submitted)
}

func TestHandleQuestionKey_QInCustomInputDoesNotCancel(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.False(t, out.Cancelled)
	assert.Equal(t, "q", s.Per[0].CustomBuffer)
}

func TestHandleQuestionKey_DClearsCustomWhenCursorOnCustomNav(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s.Per[0].CustomBuffer = "ioio"
	s.Per[0].Cursor = 2 // custom row: options 0–1, custom 2
	s.Per[0].CustomInputFocused = false

	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	assert.False(t, out.Cancelled)
	assert.False(t, out.Done)
	assert.Equal(t, "", s.Per[0].CustomBuffer)
	assert.False(t, s.Per[0].CustomSelected)
	assert.Equal(t, 1, s.Per[0].Cursor)
}

func TestHandleQuestionKey_DIgnoredWhenNotOnCustomRow(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s.Per[0].CustomBuffer = "keep"
	s.Per[0].Cursor = 0
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	assert.Equal(t, "keep", s.Per[0].CustomBuffer)
}

func TestHandleQuestionKey_CustomBufferTypingAndBackspace(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // focus custom input
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	assert.Equal(t, "hi", s.Per[0].CustomBuffer)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeySpace})
	assert.Equal(t, "hi ", s.Per[0].CustomBuffer)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "hi", s.Per[0].CustomBuffer)
}

func TestHandleQuestionKey_EnterCustomRowUsesBufferAndAutoSubmits(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // focus custom input
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, out.Done)
	assert.True(t, s.Per[0].CustomSelected)
	assert.Equal(t, 2, s.Per[0].Cursor) // custom row
	assert.Equal(t, []string{"x"}, out.Answers[0])
}

func TestHandleQuestionKey_EscInCustomInputExitsInputAndDoesNotCancel(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	require.True(t, s.Per[0].CustomInputFocused)

	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEsc})
	assert.False(t, out.Cancelled)
	assert.False(t, out.Done)
	assert.False(t, s.Submitted)
	assert.False(t, s.Per[0].CustomInputFocused)
	// Empty buffer: custom row hidden; cursor returns to last fixed option row.
	assert.Equal(t, 1, s.Per[0].Cursor)
}

func TestHandleQuestionKey_CIOFocusCustomInput(t *testing.T) {
	d := qDisplaySingle()
	for _, r := range []rune{'c', 'i', 'o'} {
		st := NewQuestionUIState(d)
		st, _ = HandleQuestionKey(d, st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		assert.True(t, st.Per[0].CustomInputFocused, string(r))
		assert.Equal(t, 2, st.Per[0].Cursor)
	}
}

func TestHandleQuestionKey_SpaceInNavSingleSelectAutoSubmits(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, out.Done)
	assert.False(t, s.Per[0].CustomInputFocused)
	assert.Equal(t, 0, s.Per[0].SingleSelected)
	assert.Equal(t, 0, s.Per[0].Cursor)
}

func TestHandleQuestionKey_HLChangesActiveTab(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.False(t, out.Done && !out.Cancelled)
	assert.Equal(t, 1, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, 2, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 1, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, 0, s.Active)
}

func TestHandleQuestionKey_SKeySubmitsFromNav(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Per[1].SingleSelected = 0 // Manually select "Yes" on second question
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	assert.True(t, out.Done)
	assert.Equal(t, []string{"Yes"}, out.Answers[1])
	assert.True(t, s.Submitted)
}

func TestHandleQuestionKey_AltEnterAndCtrlJInCustomInputAddNewline(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	assert.Equal(t, "a\nb", s.Per[0].CustomBuffer)

	s2 := NewQuestionUIState(d)
	s2, _ = HandleQuestionKey(d, s2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	s2, _ = HandleQuestionKey(d, s2, tea.KeyMsg{Type: tea.KeyCtrlJ})
	s2, _ = HandleQuestionKey(d, s2, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	assert.Equal(t, "\nz", s2.Per[0].CustomBuffer)
}

func TestHandleQuestionKey_SSubmitsEntireFormAfterTwoQuestions(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{
			Question: "Pick one",
			Options:  []string{"A", "B"},
			MultiSelect: false,
		},
		{
			Question: "Pick many",
			Options:  []string{"X", "Y"},
			MultiSelect: true,
		},
	})
	s := NewQuestionUIState(d)

	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.Equal(t, 1, s.Active)

	s, out = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	assert.True(t, out.Done)
	assert.False(t, out.Cancelled)
	assert.True(t, s.Submitted)
	require.Len(t, out.Answers, 2)
	assert.Equal(t, []string{"B"}, out.Answers[0])
	assert.Equal(t, []string(nil), out.Answers[1])
}

func TestHandleQuestionKey_EnterOnEmptyCustomInputLikeEsc(t *testing.T) {
	d := qDisplaySingle()
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	require.True(t, s.Per[0].CustomInputFocused)
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.False(t, s.Per[0].CustomInputFocused)
	assert.Equal(t, 1, s.Per[0].Cursor)
}

func TestHandleQuestionKey_LastSingleSelectAnswerOpensReview(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{
		{Question: "Q1", Options: []string{"A"}, MultiSelect: false},
		{Question: "Q2", Options: []string{"B"}, MultiSelect: false},
	})
	s := NewQuestionUIState(d)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, 1, s.Active)
	s, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, 2, s.Active)
	assert.Equal(t, 0, s.ReviewCursor)
}

func TestHandleQuestionKey_ReviewGoBackEnterOnSecondRow(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Active = 2
	s.ReviewCursor = 1
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.Equal(t, 1, s.Active)
}

func TestHandleQuestionKey_ReviewLeftGoesBack(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	s.Active = 2
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
	assert.False(t, out.Done)
	assert.Equal(t, 1, s.Active)
}

func TestAnsweredQuestionCount(t *testing.T) {
	d := qDisplayMultiTwoQuestions()
	s := NewQuestionUIState(d)
	assert.Equal(t, 0, AnsweredQuestionCount(d, s.Per))
	s.Per[0].MultiSelected[0] = true
	assert.Equal(t, 1, AnsweredQuestionCount(d, s.Per))
	s.Per[1].SingleSelected = 0
	assert.Equal(t, 2, AnsweredQuestionCount(d, s.Per))
}

func TestHandleQuestionKey_EnterOnMultiCustomNavTogglesCustomSelected(t *testing.T) {
	d := domain.NewQuestionDisplay([]domain.QuestionInfo{{
		Question: "Q", Options: []string{"A"}, MultiSelect: true,
	}})
	s := NewQuestionUIState(d)
	s.Per[0].CustomBuffer = "custom"
	s.Per[0].Cursor = 1
	s.Per[0].CustomInputFocused = false
	s, out := HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.True(t, s.Per[0].CustomSelected)
	s, out = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, out.Done)
	assert.False(t, s.Per[0].CustomSelected)
}

func TestHandleQuestionKey_PanicsOnInvalidActive(t *testing.T) {
	d := qDisplaySingle()

	// Active < 0 should never happen in correct navigation logic.
	s := QuestionUIState{Active: -1, Per: NewQuestionUIState(d).Per}
	require.Panics(t, func() {
		_, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRight})
	})

	// Single question: Active > 0 is invalid (no review step).
	s = NewQuestionUIState(d)
	s.Active = 1
	require.Panics(t, func() {
		_, _ = HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
	})

	// Two questions: valid Active is 0..2; Active 3 is invalid.
	d2 := qDisplayMultiTwoQuestions()
	s2 := NewQuestionUIState(d2)
	s2.Active = 3
	require.Panics(t, func() {
		_, _ = HandleQuestionKey(d2, s2, tea.KeyMsg{Type: tea.KeyLeft})
	})
}

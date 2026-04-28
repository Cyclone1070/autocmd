package ui

import (
	"strings"
	"unicode"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// QuestionUIState is interactive UI state for QuestionDisplay (prompt-owned; lives in ui to avoid import cycles with ToolRenderer).
type QuestionUIState struct {
	// Active is 0..len(Questions)-1 for each real question, or len(Questions) when len(Questions)>1 for the review step.
	Active int
	Per    []QuestionPerState

	// ReviewCursor is the row on the synthetic review step (0 = Submit, 1 = Go back).
	// Only used when len(Questions) > 1 and Active == len(Questions).
	ReviewCursor int

	// Submitted locks the question interaction after the user submits, cancels (Esc/q), etc.
	// Until the tool ends, we ignore subsequent keypresses to avoid duplicate actions.
	Submitted bool
}

// QuestionPerState is state for one question in the toolbox.
type QuestionPerState struct {
	Cursor             int
	MultiSelected      []bool
	CustomBuffer       string
	CustomInputFocused bool
	SingleSelected     int
	CustomSelected     bool
}

// QuestionOutcome is the reducer result when the user submits or cancels.
type QuestionOutcome struct {
	Done      bool
	Cancelled bool
	Answers   [][]string
}

// NewQuestionUIState initializes state for d.Questions (one QuestionPerState each).
func NewQuestionUIState(d domain.QuestionDisplay) QuestionUIState {
	per := make([]QuestionPerState, len(d.Questions))
	for i := range d.Questions {
		q := d.Questions[i]
		n := len(q.Options)
		st := QuestionPerState{
			Cursor:         0,
			MultiSelected:  make([]bool, n),
			SingleSelected: -1,
		}
		if n == 0 {
			st.CustomInputFocused = true
		}
		per[i] = st
	}
	return QuestionUIState{Active: 0, Per: per, ReviewCursor: 0, Submitted: false}
}

// maxActiveIndex is the largest valid Active: last question index, or len(Questions) when a review step exists (n > 1).
func maxActiveIndex(d domain.QuestionDisplay) int {
	n := len(d.Questions)
	if n <= 1 {
		return n - 1
	}
	return n
}

// OnReview reports whether the UI is on the synthetic review step (Active == n with n > 1).
func OnReview(d domain.QuestionDisplay, s QuestionUIState) bool {
	n := len(d.Questions)
	return n > 1 && s.Active == n
}

// QuestionHasAnswer is true if the question has a non-empty selection or custom text.
func QuestionHasAnswer(q domain.QuestionInfo, st QuestionPerState) bool {
	if q.MultiSelect {
		for _, on := range st.MultiSelected {
			if on {
				return true
			}
		}
		return st.CustomSelected && strings.TrimSpace(st.CustomBuffer) != ""
	}
	optsLen := len(q.Options)
	if st.SingleSelected >= 0 && st.SingleSelected < optsLen {
		return true
	}
	return st.CustomSelected && strings.TrimSpace(st.CustomBuffer) != ""
}

// AnsweredQuestionCount returns how many questions have a non-empty answer (for the review summary).
func AnsweredQuestionCount(d domain.QuestionDisplay, per []QuestionPerState) int {
	k := 0
	for i := range d.Questions {
		if i < len(per) && QuestionHasAnswer(d.Questions[i], per[i]) {
			k++
		}
	}
	return k
}

// UnansweredIndices returns the 1-based indices of questions without answers.
func UnansweredIndices(d domain.QuestionDisplay, per []QuestionPerState) []int {
	var out []int
	for i := range d.Questions {
		if i < len(per) && !QuestionHasAnswer(d.Questions[i], per[i]) {
			out = append(out, i+1)
		}
	}
	return out
}

// customRowVisible is true when the free-text row is shown (always available in the UI layer).
func customRowVisible(st QuestionPerState) bool {
	return st.CustomInputFocused || strings.TrimSpace(st.CustomBuffer) != ""
}

func customRowIndex(q domain.QuestionInfo) int {
	return len(q.Options)
}

func numQuestionRows(q domain.QuestionInfo, st QuestionPerState) int {
	n := len(q.Options)
	if customRowVisible(st) {
		return n + 1
	}
	return n
}

// lastRowIndex is the highest cursor index (options and optional custom row; no separate submit row).
func lastRowIndex(q domain.QuestionInfo, st QuestionPerState) int {
	return numQuestionRows(q, st) - 1
}

func clampQuestionCursor(q domain.QuestionInfo, st *QuestionPerState) {
	limit := lastRowIndex(q, *st)
	if st.Cursor > limit {
		st.Cursor = limit
	}
	if st.Cursor < 0 {
		st.Cursor = 0
	}
}

// applyExitCustomLikeEsc leaves custom edit mode; empty buffer moves the cursor to the last fixed option.
func applyExitCustomLikeEsc(q domain.QuestionInfo, st *QuestionPerState) {
	st.CustomInputFocused = false
	if strings.TrimSpace(st.CustomBuffer) == "" {
		if len(q.Options) > 0 {
			st.Cursor = len(q.Options) - 1
		} else {
			st.Cursor = 0
		}
	} else {
		clampQuestionCursor(q, st)
	}
}

// advanceAfterSingleAnswer moves to the next question, or to the review step when finishing the last question (n > 1).
// If there is only one question (and it's not multi-select, which is handled upstream), it auto-submits.
func advanceAfterSingleAnswer(d domain.QuestionDisplay, s *QuestionUIState) (QuestionUIState, QuestionOutcome) {
	last := len(d.Questions) - 1
	if s.Active < last {
		s.Active++
		return *s, QuestionOutcome{}
	}
	if len(d.Questions) == 1 {
		s.Submitted = true
		return *s, buildSubmitOutcome(d, *s)
	}
	if len(d.Questions) > 1 && s.Active == last {
		s.Active = len(d.Questions)
		s.ReviewCursor = 0
	}
	return *s, QuestionOutcome{}
}

// questionPrimaryAction handles Enter / Space in navigation (and Enter committing custom input).
// Form submit is only via the "s" key (not Enter on a submit row).
func questionPrimaryAction(d domain.QuestionDisplay, s QuestionUIState) (QuestionUIState, QuestionOutcome) {
	q := d.Questions[s.Active]
	st := &s.Per[s.Active]
	customIx := customRowIndex(q)

	onCustom := st.CustomInputFocused || (customRowVisible(*st) && st.Cursor == customIx)
	if onCustom {
		if strings.TrimSpace(st.CustomBuffer) == "" {
			applyExitCustomLikeEsc(q, st)
			return s, QuestionOutcome{}
		}
		if q.MultiSelect {
			if st.CustomInputFocused {
				st.SingleSelected = -1
				st.CustomSelected = true
				st.CustomInputFocused = false
				clampQuestionCursor(q, st)
				return s, QuestionOutcome{}
			}
			st.SingleSelected = -1
			st.CustomSelected = !st.CustomSelected
			return s, QuestionOutcome{}
		}
		// Single-select custom
		if st.CustomInputFocused {
			st.SingleSelected = -1
			st.CustomSelected = true
			st.CustomInputFocused = false
			clampQuestionCursor(q, st)
			return advanceAfterSingleAnswer(d, &s)
		}
		if st.CustomSelected {
			st.CustomSelected = false
			st.SingleSelected = -1
			return s, QuestionOutcome{}
		}
		st.SingleSelected = -1
		st.CustomSelected = true
		return advanceAfterSingleAnswer(d, &s)
	}

	if st.Cursor < 0 || st.Cursor >= len(q.Options) {
		return s, QuestionOutcome{}
	}
	if q.MultiSelect {
		st.MultiSelected[st.Cursor] = !st.MultiSelected[st.Cursor]
		return s, QuestionOutcome{}
	}
	if st.SingleSelected == st.Cursor {
		st.SingleSelected = -1
		st.CustomSelected = false
		return s, QuestionOutcome{}
	}
	st.SingleSelected = st.Cursor
	st.CustomSelected = false
	return advanceAfterSingleAnswer(d, &s)
}

func handleReviewKey(d domain.QuestionDisplay, s QuestionUIState, key tea.KeyMsg) (QuestionUIState, QuestionOutcome) {
	n := len(d.Questions)
	switch key.Type {
	case tea.KeyEsc:
		s.Submitted = true
		return s, QuestionOutcome{Cancelled: true}
	case tea.KeyEnter, tea.KeySpace:
		if s.ReviewCursor == 0 {
			s.Submitted = true
			return s, buildSubmitOutcome(d, s)
		}
		s.Active = n - 1
		return s, QuestionOutcome{}
	case tea.KeyCtrlP, tea.KeyUp, tea.KeyShiftTab:
		if s.ReviewCursor > 0 {
			s.ReviewCursor--
		} else {
			s.ReviewCursor = 1
		}
		return s, QuestionOutcome{}
	case tea.KeyCtrlN, tea.KeyDown, tea.KeyTab:
		if s.ReviewCursor < 1 {
			s.ReviewCursor++
		} else {
			s.ReviewCursor = 0
		}
		return s, QuestionOutcome{}
	case tea.KeyLeft:
		s.Active = n - 1
		return s, QuestionOutcome{}
	case tea.KeyRight:
		return s, QuestionOutcome{}
	default:
		if key.Type != tea.KeyRunes || len(key.Runes) != 1 {
			return s, QuestionOutcome{}
		}
		switch key.Runes[0] {
		case 'j':
			return handleReviewKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
		case 'k':
			return handleReviewKey(d, s, tea.KeyMsg{Type: tea.KeyUp})
		case 'h':
			return handleReviewKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
		case 'l':
			return handleReviewKey(d, s, tea.KeyMsg{Type: tea.KeyRight})
		case 'q':
			s.Submitted = true
			return s, QuestionOutcome{Cancelled: true}
		case 's':
			s.Submitted = true
			return s, buildSubmitOutcome(d, s)
		}
		return s, QuestionOutcome{}
	}
}

// HandleQuestionKey applies one logical key to state and returns updated state and outcome.
func HandleQuestionKey(d domain.QuestionDisplay, s QuestionUIState, key tea.KeyMsg) (QuestionUIState, QuestionOutcome) {
	if len(d.Questions) == 0 {
		panic("HandleQuestionKey: QuestionDisplay has no questions")
	}
	maxA := maxActiveIndex(d)
	if s.Active < 0 || s.Active > maxA {
		panic("HandleQuestionKey: QuestionUIState.Active out of range")
	}

	if s.Submitted {
		return s, QuestionOutcome{}
	}

	if OnReview(d, s) {
		return handleReviewKey(d, s, key)
	}

	// Per is initialized by NewQuestionUIState and must stay consistent with d.Questions length.
	if s.Active >= len(s.Per) {
		panic("HandleQuestionKey: QuestionUIState.Per out of range for Active")
	}

	q := d.Questions[s.Active]
	st := &s.Per[s.Active]
	customIx := customRowIndex(q)

	if key.Type == tea.KeyRunes && st.CustomInputFocused {
		for _, r := range key.Runes {
			if !unicode.IsPrint(r) {
				continue
			}
			st.CustomBuffer += string(r)
		}
		return s, QuestionOutcome{}
	}

	switch key.Type {
	case tea.KeyEsc:
		if st.CustomInputFocused {
			applyExitCustomLikeEsc(q, st)
			return s, QuestionOutcome{}
		}
		s.Submitted = true
		return s, QuestionOutcome{Cancelled: true}
	case tea.KeyCtrlJ:
		// Line feed (some terminals send this for modified Enter); use as newline in custom input.
		if st.CustomInputFocused {
			st.CustomBuffer += "\n"
			return s, QuestionOutcome{}
		}
		return s, QuestionOutcome{}
	case tea.KeyEnter:
		// Alt+Enter is a newline surrogate (many terminals cannot distinguish Shift+Enter).
		if st.CustomInputFocused && key.Alt {
			st.CustomBuffer += "\n"
			return s, QuestionOutcome{}
		}
		return questionPrimaryAction(d, s)
	case tea.KeyLeft:
		if st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		if s.Active > 0 {
			s.Active--
		}
		return s, QuestionOutcome{}
	case tea.KeyRight:
		if st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		n := len(d.Questions)
		if s.Active < n-1 {
			s.Active++
		} else if n > 1 {
			s.Active = n
			s.ReviewCursor = 0
		}
		return s, QuestionOutcome{}
	case tea.KeyCtrlP, tea.KeyUp, tea.KeyShiftTab:
		if st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		maxC := lastRowIndex(q, *st)
		if st.Cursor > 0 {
			st.Cursor--
		} else {
			st.Cursor = maxC
		}
		return s, QuestionOutcome{}
	case tea.KeyCtrlN, tea.KeyDown, tea.KeyTab:
		if st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		maxC := lastRowIndex(q, *st)
		if st.Cursor < maxC {
			st.Cursor++
		} else {
			st.Cursor = 0
		}
		return s, QuestionOutcome{}
	case tea.KeySpace:
		if st.CustomInputFocused {
			st.CustomBuffer += " "
			return s, QuestionOutcome{}
		}
		return questionPrimaryAction(d, s)
	case tea.KeyBackspace:
		if !st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		if len(st.CustomBuffer) > 0 {
			st.CustomBuffer = st.CustomBuffer[:len(st.CustomBuffer)-1]
		}
		return s, QuestionOutcome{}
	default:
		if st.CustomInputFocused {
			return s, QuestionOutcome{}
		}
		if key.Type != tea.KeyRunes || len(key.Runes) != 1 {
			return s, QuestionOutcome{}
		}
		switch key.Runes[0] {
		case 'j':
			return HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyDown})
		case 'k':
			return HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyUp})
		case 'h':
			return HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyLeft})
		case 'l':
			return HandleQuestionKey(d, s, tea.KeyMsg{Type: tea.KeyRight})
		case 'q':
			s.Submitted = true
			return s, QuestionOutcome{Cancelled: true}
		case 's':
			s.Submitted = true
			return s, buildSubmitOutcome(d, s)
		case 'd':
			if st.Cursor == customIx && !st.CustomInputFocused && customRowVisible(*st) {
				st.CustomBuffer = ""
				st.CustomSelected = false
				if len(q.Options) > 0 {
					st.Cursor = len(q.Options) - 1
				} else {
					st.Cursor = 0
				}
				clampQuestionCursor(q, st)
				return s, QuestionOutcome{}
			}
			return s, QuestionOutcome{}
		case 'c', 'i', 'o':
			st.Cursor = customIx
			st.CustomInputFocused = true
			return s, QuestionOutcome{}
		}
		return s, QuestionOutcome{}
	}
}

func buildSubmitOutcome(d domain.QuestionDisplay, s QuestionUIState) QuestionOutcome {
	answers := make([][]string, len(d.Questions))
	for qi, q := range d.Questions {
		st := s.Per[qi]
		optsLen := len(q.Options)

		if q.MultiSelect {
			var labels []string
			for j, on := range st.MultiSelected {
				if j < optsLen && on {
					labels = append(labels, q.Options[j])
				}
			}
			if st.CustomSelected {
				if cust := strings.TrimSpace(st.CustomBuffer); cust != "" {
					labels = append(labels, cust)
				}
			}
			answers[qi] = labels
			continue
		}

		// Single select
		if st.SingleSelected >= 0 && st.SingleSelected < optsLen {
			answers[qi] = []string{q.Options[st.SingleSelected]}
			continue
		}
		if st.CustomSelected {
			if cust := strings.TrimSpace(st.CustomBuffer); cust != "" {
				answers[qi] = []string{cust}
			} else {
				answers[qi] = nil
			}
			continue
		}
		answers[qi] = nil
	}
	return QuestionOutcome{Done: true, Answers: answers}
}

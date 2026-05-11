package question

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingBus struct {
	updates []domain.UIUpdate
}

func (b *capturingBus) SendUIUpdate(u domain.UIUpdate) { b.updates = append(b.updates, u) }

func TestTool(t *testing.T) {
	tool := NewTool()

	t.Run("Name and Definition", func(t *testing.T) {
		assert.Equal(t, "ask_question", tool.Name())
		assert.False(t, tool.IsConcurrentSafe())
		def, err := tool.Info(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, def)
		assert.Equal(t, "ask_question", def.Name)
	})

	t.Run("Prepare and Display", func(t *testing.T) {
		params := `{"questions": [{"question": "Color?", "options": ["Red", "Blue"]}]}`
		questions, err := tool.validate(params)
		assert.NoError(t, err)

		qd := domain.NewQuestionDisplay(questions)
		assert.Len(t, qd.Questions, 1)
		assert.Equal(t, "Color?", qd.Questions[0].Question)
	})

	t.Run("Prepare with MultiSelect", func(t *testing.T) {
		params := `{"questions": [{"question": "Colors?", "options": ["Red", "Blue"], "multiSelect": true}]}`
		questions, err := tool.validate(params)
		assert.NoError(t, err)

		qd := domain.NewQuestionDisplay(questions)
		assert.True(t, qd.Questions[0].MultiSelect)
	})

	t.Run("Resolve and Format", func(t *testing.T) {
		params := `{"questions": [{"question": "Pick name?"}]}`
		questions, _ := tool.validate(params)

		act := domain.QuestionAnswerAction{
			Answers: [][]string{{"Alice"}},
		}

		llmContent, finalDisplay := resolveQuestions(context.Background(), questions, act)
		assert.Contains(t, llmContent, "Alice")
		assert.Contains(t, llmContent, "User has answered your questions (0 skipped):\n\nQ: Pick name?\nA: Alice\n\nYou can now continue with the user's answers in mind.")

		sd, ok := finalDisplay.(domain.StringDisplay)
		assert.True(t, ok)
		assert.Equal(t, "Questions answered", sd.Description)
		assert.Contains(t, sd.Content, "Alice")
	})

	t.Run("Resolve omits skipped questions", func(t *testing.T) {
		params := `{"questions": [{"question": "Q1?"}, {"question": "Q2?"}]}`
		questions, _ := tool.validate(params)

		act := domain.QuestionAnswerAction{
			Answers: [][]string{{"A1"}, {}},
		}

		llmContent, finalDisplay := resolveQuestions(context.Background(), questions, act)
		assert.Contains(t, llmContent, "User has answered your questions (1 skipped):\n\nQ: Q1?\nA: A1\n\nYou can now continue with the user's answers in mind.")
		assert.NotContains(t, llmContent, "Q: Q2?")
		assert.NotContains(t, llmContent, "Not answered")

		sd, ok := finalDisplay.(domain.StringDisplay)
		assert.True(t, ok)
		assert.Equal(t, "Questions answered", sd.Description)
		assert.Contains(t, sd.Content, "Q: Q1?\nA: A1")
		assert.NotContains(t, sd.Content, "Q: Q2?")
		assert.NotContains(t, sd.Content, "Not answered")
	})

	t.Run("Resolve all skipped returns skipped message and display", func(t *testing.T) {
		params := `{"questions": [{"question": "Q1?"}, {"question": "Q2?"}]}`
		questions, _ := tool.validate(params)

		act := domain.QuestionAnswerAction{
			Answers: [][]string{{}, {}},
		}

		llmContent, finalDisplay := resolveQuestions(context.Background(), questions, act)
		assert.Equal(t, "User has skipped all questions.", llmContent)

		sd, ok := finalDisplay.(domain.StringDisplay)
		require.True(t, ok)
		assert.Equal(t, "Questions skipped", sd.Description)
		assert.Equal(t, "", sd.Content)
		assert.Equal(t, "", sd.Error)
	})

	t.Run("Resolve Cancellation", func(t *testing.T) {
		params := `{"questions": [{"question": "Color?"}, {"question": "Size?"}]}`
		questions, _ := tool.validate(params)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		llmContent, finalDisplay := resolveQuestions(ctx, questions, nil)
		assert.Error(t, ctx.Err())
		assert.Equal(t, context.Canceled, ctx.Err())
		assert.Equal(t, domain.ToolErrorCancelled, llmContent)

		sd, ok := finalDisplay.(domain.StringDisplay)
		require.True(t, ok)
		assert.Equal(t, domain.ToolErrorCancelled, sd.Error)
		assert.Equal(t, "Question attempted", sd.Description)
		assert.Equal(t, "", sd.Content)
	})

	t.Run("InvokableRun without waiter emits ToolEnd", func(t *testing.T) {
		bus := &capturingBus{}
		ctx := runtimectx.WithEventSender(context.Background(), bus)

		_, err := tool.InvokableRun(ctx, `{"questions":[{"question":"Color?","options":["Red","Blue"]}]}`)
		require.Error(t, err)
		require.Len(t, bus.updates, 1)
		end, ok := bus.updates[0].(domain.ToolEndEvent)
		require.True(t, ok)
		require.Equal(t, domain.ToolErrorFailed, end.Display.GetError())
		sd, ok := end.Display.(domain.StringDisplay)
		require.True(t, ok)
		require.Equal(t, "Question attempted", sd.Description)
	})
}

func TestTool_Resolve_PanicsOnUnexpectedAction(t *testing.T) {
	tool := NewTool()
	params := `{"questions": [{"question": "Q"}]}`
	questions, _ := tool.validate(params)

	assert.Panics(t, func() {
		resolveQuestions(context.Background(), questions, domain.StopAction{})
	})
}

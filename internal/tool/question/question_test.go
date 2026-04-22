package question

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuestionTool(t *testing.T) {
	tool := NewQuestionTool()

	t.Run("Name and Definition", func(t *testing.T) {
		assert.Equal(t, "ask_question", tool.Name())
		assert.False(t, tool.IsConcurrentSafe())
		def := tool.Definition()
		assert.NotNil(t, def)
		assert.Equal(t, "ask_question", def.Name)
	})

	t.Run("Prepare and Display", func(t *testing.T) {
		params := `{"questions": [{"question": "Color?", "options": ["Red", "Blue"]}]}`
		inv, err := tool.Prepare(params)
		assert.NoError(t, err)

		d := inv.Display()
		qd, ok := d.(domain.QuestionDisplay)
		assert.True(t, ok)
		assert.Len(t, qd.Questions, 1)
		assert.Equal(t, "Color?", qd.Questions[0].Question)
	})

	t.Run("Prepare with MultiSelect", func(t *testing.T) {
		params := `{"questions": [{"question": "Colors?", "options": ["Red", "Blue"], "multiSelect": true}]}`
		inv, err := tool.Prepare(params)
		assert.NoError(t, err)

		d := inv.Display()
		qd, ok := d.(domain.QuestionDisplay)
		assert.True(t, ok)
		assert.True(t, qd.Questions[0].MultiSelect)
	})

	t.Run("Resolve and Format", func(t *testing.T) {
		params := `{"questions": [{"question": "Pick name?"}]}`
		inv, _ := tool.Prepare(params)
		ii, _ := inv.(domain.InteractiveInvocation)

		act := domain.QuestionAnswerAction{
			Answers: [][]string{{"Alice"}},
		}

		llmContent, finalDisplay := ii.Resolve(context.Background(), act)
		assert.Contains(t, llmContent, "Alice")
		assert.Contains(t, llmContent, "Q: Pick name?\nA: Alice")

		sd, ok := finalDisplay.(domain.StringDisplay)
		assert.True(t, ok)
		assert.Equal(t, "Questions attempted", sd.Description)
		assert.Contains(t, sd.Content, "Alice")
	})

	t.Run("Resolve Cancellation", func(t *testing.T) {
		params := `{"questions": [{"question": "Color?"}, {"question": "Size?"}]}`
		inv, _ := tool.Prepare(params)
		ii, _ := inv.(domain.InteractiveInvocation)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		llmContent, finalDisplay := ii.Resolve(ctx, nil)
		assert.Error(t, ctx.Err())
		assert.Equal(t, context.Canceled, ctx.Err())
		assert.Equal(t, domain.ToolErrorCancelled, llmContent)

		sd, ok := finalDisplay.(domain.StringDisplay)
		require.True(t, ok)
		assert.Equal(t, domain.ToolErrorCancelled, sd.Error)
		assert.Equal(t, "Questions attempted", sd.Description)
		assert.Equal(t, "Q: Color?\n\nQ: Size?", sd.Content)
	})
}

func TestQuestionTool_Resolve_PanicsOnUnexpectedAction(t *testing.T) {
	tool := NewQuestionTool()
	params := `{"questions": [{"question": "Q"}]}`
	inv, _ := tool.Prepare(params)
	ii, _ := inv.(domain.InteractiveInvocation)

	assert.Panics(t, func() {
		ii.Resolve(context.Background(), domain.StopAction{})
	})
}

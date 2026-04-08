package question

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
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
		assert.True(t, qd.Questions[0].MultiSelect) // This will fail to compile initially
	})

	t.Run("Resolve and Format", func(t *testing.T) {
		params := `{"questions": [{"question": "Pick name?"}]}`
		inv, _ := tool.Prepare(params)
		ii, _ := inv.(domain.InteractiveInvocation)

		act := domain.QuestionAnswerAction{
			Answers: [][]string{{"Alice"}},
		}
		
		llmContent, finalDisplay, err := ii.Resolve(context.Background(), act)
		assert.NoError(t, err)
		assert.Contains(t, llmContent, "Alice")
		assert.Contains(t, llmContent, "Q: Pick name?\nA: Alice")

		sd, ok := finalDisplay.(domain.StringDisplay)
		assert.True(t, ok)
		assert.Contains(t, sd.Content, "Alice")
	})
}

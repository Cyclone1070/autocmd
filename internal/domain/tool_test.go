package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolDisplays_UnmarshalJSON(t *testing.T) {
	t.Run("StringDisplay with comment", func(t *testing.T) {
		data := `{
			"call-1": {
				"type": "string",
				"content": "some content",
				"comment": "some comment"
			}
		}`

		var m ToolDisplays
		err := json.Unmarshal([]byte(data), &m)
		assert.NoError(t, err)

		d, ok := m["call-1"].(StringDisplay)
		assert.True(t, ok)
		assert.Equal(t, "some content", d.Content)
		assert.Equal(t, "some comment", d.Comment)
	})
	t.Run("QuestionDisplay", func(t *testing.T) {
		data := `{
			"call-1": {
				"type": "question",
				"questions": [
					{
						"question": "Pick one?",
						"options": ["A", "B"],
						"multiSelect": true
					}
				]
			}
		}`

		var m ToolDisplays
		err := json.Unmarshal([]byte(data), &m)
		require.NoError(t, err)

		d, ok := m["call-1"].(QuestionDisplay)
		require.True(t, ok)
		assert.Equal(t, "question", d.Type())
		assert.Len(t, d.Questions, 1)
		assert.Equal(t, "Pick one?", d.Questions[0].Question)
		assert.True(t, d.Questions[0].MultiSelect)
		assert.Len(t, d.Questions[0].Options, 2)
		assert.Equal(t, "A", d.Questions[0].Options[0])
	})
}

func TestNewQuestionDisplay(t *testing.T) {
	d := NewQuestionDisplay([]QuestionInfo{
		{Question: "Q?", Options: []string{"x"}},
	})
	assert.Equal(t, "question", d.TypeField)
	assert.Len(t, d.Questions, 1)
}

func TestQuestionAnswerAction_GetCallID(t *testing.T) {
	a := QuestionAnswerAction{CallID: "tc-1", Answers: [][]string{{"a"}}}
	var c CallIDer = a
	assert.Equal(t, "tc-1", c.GetCallID())
}

func TestToolDisplay_GetError(t *testing.T) {
	t.Run("StringDisplay", func(t *testing.T) {
		d := NewStringDisplay("c", "x")
		var td ToolDisplay = d
		assert.Equal(t, "", td.GetError())
		d2 := StringDisplay{TypeField: "string", Comment: "c", Content: "x", Error: "e"}
		assert.Equal(t, "e", d2.GetError())
	})
	t.Run("DiffDisplay", func(t *testing.T) {
		d := NewDiffDisplay("c", "t", 1, 2, "diff")
		var td ToolDisplay = d
		assert.Equal(t, "", td.GetError())
		d2 := DiffDisplay{TypeField: "diff", Comment: "c", Target: "t", Diff: "d", Error: "bad"}
		assert.Equal(t, "bad", d2.GetError())
	})
	t.Run("BashDisplay", func(t *testing.T) {
		d := NewBashDisplay("c", "cmd", "out")
		var td ToolDisplay = d
		assert.Equal(t, "", td.GetError())
		d2 := BashDisplay{TypeField: "bash", Comment: "c", Command: "cmd", Error: "x"}
		assert.Equal(t, "x", d2.GetError())
	})
	t.Run("QuestionDisplay", func(t *testing.T) {
		d := NewQuestionDisplay(nil)
		var td ToolDisplay = d
		assert.Equal(t, "", td.GetError())
		d2 := QuestionDisplay{TypeField: "question", Questions: nil, Error: "e"}
		assert.Equal(t, "e", d2.GetError())
	})
}

func TestToolDisplay_WithError(t *testing.T) {
	t.Run("StringDisplay", func(t *testing.T) {
		base := NewStringDisplay("c", "x")
		got := base.WithError("boom").(StringDisplay)
		assert.Equal(t, "boom", got.Error)
		assert.Equal(t, "", base.Error, "WithError must not mutate original")
	})
	t.Run("DiffDisplay", func(t *testing.T) {
		base := NewDiffDisplay("c", "t", 1, 2, "diff")
		got := base.WithError("boom").(DiffDisplay)
		assert.Equal(t, "boom", got.Error)
		assert.Equal(t, "", base.Error, "WithError must not mutate original")
	})
	t.Run("BashDisplay", func(t *testing.T) {
		base := NewBashDisplay("c", "cmd", "out")
		got := base.WithError("boom").(BashDisplay)
		assert.Equal(t, "boom", got.Error)
		assert.Equal(t, "", base.Error, "WithError must not mutate original")
	})
	t.Run("QuestionDisplay", func(t *testing.T) {
		base := NewQuestionDisplay(nil)
		got := base.WithError("boom").(QuestionDisplay)
		assert.Equal(t, "boom", got.Error)
		assert.Equal(t, "", base.Error, "WithError must not mutate original")
	})
}

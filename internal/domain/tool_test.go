package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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
	t.Run("ShellDisplay", func(t *testing.T) {
		d := NewShellDisplay("c", "cmd", "out")
		var td ToolDisplay = d
		assert.Equal(t, "", td.GetError())
		d2 := ShellDisplay{TypeField: "shell", Comment: "c", Command: "cmd", Error: "x"}
		assert.Equal(t, "x", d2.GetError())
	})
}

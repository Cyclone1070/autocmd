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

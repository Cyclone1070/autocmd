package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestAppendMessageMerge_EmptyThenUser(t *testing.T) {
	var msgs []*schema.Message
	require.NoError(t, appendMessageMerge(&msgs, schema.UserMessage("hello")))
	require.Len(t, msgs, 1)
	require.Equal(t, schema.User, msgs[0].Role)
	require.Equal(t, "hello", msgs[0].Content)
}

func TestAppendMessageMerge_UserUserMerges(t *testing.T) {
	msgs := []*schema.Message{schema.UserMessage("a")}
	require.NoError(t, appendMessageMerge(&msgs, schema.UserMessage("b")))
	require.Len(t, msgs, 1)
	require.Equal(t, schema.User, msgs[0].Role)
	require.Equal(t, "a\n\nb", msgs[0].Content)
}

func TestAppendMessageMerge_UserUserTrimsAndSeparatesWithBlankLine(t *testing.T) {
	msgs := []*schema.Message{schema.UserMessage("  a  ")}
	require.NoError(t, appendMessageMerge(&msgs, schema.UserMessage("\nb\t")))
	require.Len(t, msgs, 1)
	require.Equal(t, "a\n\nb", msgs[0].Content)
}

func TestAppendMessageMerge_AssistantAssistantMerges(t *testing.T) {
	msgs := []*schema.Message{schema.AssistantMessage("x", nil)}
	require.NoError(t, appendMessageMerge(&msgs, schema.AssistantMessage("y", nil)))
	require.Len(t, msgs, 1)
	require.Equal(t, schema.Assistant, msgs[0].Role)
	require.Equal(t, "x\n\ny", msgs[0].Content)
}

func TestAppendMessageMerge_ToolNeverMergesWithPriorTool(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.Tool, Content: "out1", ToolCallID: "1"},
	}
	require.NoError(t, appendMessageMerge(&msgs, &schema.Message{Role: schema.Tool, Content: "out2", ToolCallID: "2"}))
	require.Len(t, msgs, 2)
	require.Equal(t, "out1", msgs[0].Content)
	require.Equal(t, "out2", msgs[1].Content)
}

func TestAppendMessageMerge_UserThenToolAppends(t *testing.T) {
	msgs := []*schema.Message{schema.UserMessage("q")}
	require.NoError(t, appendMessageMerge(&msgs, &schema.Message{Role: schema.Tool, Content: "t", ToolCallID: "id"}))
	require.Len(t, msgs, 2)
	require.Equal(t, schema.User, msgs[0].Role)
	require.Equal(t, schema.Tool, msgs[1].Role)
}

func TestAppendMessageMerge_NilMessageErrors(t *testing.T) {
	var msgs []*schema.Message
	require.Error(t, appendMessageMerge(&msgs, nil))
}

func TestAppendMessageMerge_MergePreservesIncomingExtraOnConflict(t *testing.T) {
	msgs := []*schema.Message{{
		Role:    schema.User,
		Content: "a",
		Extra:   map[string]any{"k": 1},
	}}
	require.NoError(t, appendMessageMerge(&msgs, &schema.Message{
		Role:    schema.User,
		Content: "b",
		Extra:   map[string]any{"k": 2, "j": 3},
	}))
	require.Len(t, msgs, 1)
	require.Equal(t, "a\n\nb", msgs[0].Content)
	require.Equal(t, 2, msgs[0].Extra["k"])
	require.Equal(t, 3, msgs[0].Extra["j"])
}

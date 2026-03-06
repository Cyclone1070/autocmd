package google

import (
	"encoding/json"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestToTools(t *testing.T) {
	decls := []domain.Declaration{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters: &domain.Schema{
				Type: domain.TypeObject,
				Properties: map[string]*domain.Schema{
					"arg1": {Type: domain.TypeString},
				},
			},
		},
	}

	tools := toTools(decls)
	assert.Len(t, tools, 1)
	fd := tools[0].FunctionDeclarations[0]
	assert.Equal(t, "test_tool", fd.Name)
	assert.Equal(t, "A test tool", fd.Description)
	assert.Equal(t, genai.TypeObject, fd.Parameters.Type)
	assert.Equal(t, genai.TypeString, fd.Parameters.Properties["arg1"].Type)
}

func TestToHistory(t *testing.T) {
	// Test basic case (system + user)
	msgs := []domain.Message{
		domain.SystemMessage{Content: "Be helpful"},
		domain.UserMessage{Content: "Hi"},
	}

	hist, err := toHistory(msgs)
	if err != nil {
		t.Fatalf("toHistory failed: %v", err)
	}

	if hist.SystemPrompt != "Be helpful" {
		t.Errorf("expected system prompt 'Be helpful', got %q", hist.SystemPrompt)
	}

	if len(hist.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(hist.Contents))
	}
	if hist.Contents[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", hist.Contents[0].Role)
	}
	if hist.Contents[0].Parts[0].Text != "Hi" {
		t.Errorf("expected text 'Hi', got %q", hist.Contents[0].Parts[0].Text)
	}

	// Test roles and multiple turns
	msgs = []domain.Message{
		domain.UserMessage{Content: "A"},      // 0
		domain.AssistantMessage{Content: "B"}, // 1
		domain.UserMessage{Content: "C"},      // 2
	}

	hist, err = toHistory(msgs)
	if err != nil {
		t.Fatalf("toHistory failed: %v", err)
	}

	if len(hist.Contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(hist.Contents))
	}

	// Check roles
	if hist.Contents[0].Role != "user" {
		t.Errorf("msg[0] role mismatch")
	}
	if hist.Contents[1].Role != "model" {
		t.Errorf("msg[1] role mismatch: expected 'model', got %q", hist.Contents[1].Role)
	}
	if hist.Contents[2].Role != "user" {
		t.Errorf("msg[2] role mismatch")
	}

	// Check content
	if hist.Contents[1].Parts[0].Text != "B" {
		t.Errorf("msg[1] content mismatch")
	}
}

func TestToHistory_ToolCall(t *testing.T) {
	args := json.RawMessage(`{"location":"Paris"}`)
	msgs := []domain.Message{
		domain.AssistantMessage{
			Content: "",
			ToolCalls: []domain.ToolCall{
				{ID: "tc-1", Name: "get_weather", Arguments: args},
			},
		},
		domain.ToolMessage{
			ToolCallID: "tc-1",
			ToolName:   "get_weather",
			Content:    "Sunny",
		},
	}

	hist, err := toHistory(msgs)
	if err != nil {
		t.Fatalf("toHistory failed: %v", err)
	}

	if len(hist.Contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(hist.Contents))
	}

	// 1. Tool Call
	if hist.Contents[0].Role != "model" {
		t.Errorf("msg[0] role mismatch: expected 'model', got %q", hist.Contents[0].Role)
	}
	// Verify raw parts structure for tool call
	parts := hist.Contents[0].Parts
	if len(parts) != 1 || parts[0].FunctionCall == nil {
		t.Fatal("expected function call in msg[0]")
	}
	if parts[0].FunctionCall.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", parts[0].FunctionCall.Name)
	}
	assert.Equal(t, "Paris", parts[0].FunctionCall.Args["location"])

	// 2. Tool Response
	if hist.Contents[1].Role != "function" {
		t.Errorf("msg[1] role mismatch: expected 'function', got %q", hist.Contents[1].Role)
	}
	parts = hist.Contents[1].Parts
	if len(parts) != 1 || parts[0].FunctionResponse == nil {
		t.Fatal("expected function response in msg[1]")
	}
	if parts[0].FunctionResponse.Name != "get_weather" {
		t.Errorf("expected response name 'get_weather', got %q", parts[0].FunctionResponse.Name)
	}
	assert.Equal(t, "Sunny", parts[0].FunctionResponse.Response["result"])
}

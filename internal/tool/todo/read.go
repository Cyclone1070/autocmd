package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/iav/internal/tool"
)

// ReadTodosTool handles reading todos.
type ReadTodosTool struct {
	store todoStore
}

// NewReadTodosTool creates a new ReadTodosTool with injected dependencies.
func NewReadTodosTool(store todoStore) *ReadTodosTool {
	if store == nil {
		panic("store is required")
	}
	return &ReadTodosTool{
		store: store,
	}
}

// Name returns the tool's identifier.
func (t *ReadTodosTool) Name() string {
	return "todoread"
}

// Declaration returns the tool's schema for the LLM.
func (t *ReadTodosTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        "todoread",
		Description: "Read the current list of todos. Returns a JSON array of todo items.",
		Parameters: &tool.Schema{
			Type:       tool.TypeObject,
			Properties: map[string]*tool.Schema{},
			Required:   []string{},
		},
	}
}

// Prepare validates the request and returns an Invocation.
func (t *ReadTodosTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	// No validation needed for read - just return the invocation
	return &readTodosInvocation{
		store:   t.store,
		display: tool.StringDisplay("Reading todos"),
	}, nil
}

type readTodosInvocation struct {
	store   todoStore
	display tool.ToolDisplay
}

func (i *readTodosInvocation) Display() tool.ToolDisplay {
	return i.display
}

func (i *readTodosInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	todos, err := i.store.Read()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to read todos: %v", err), err
	}

	// Return empty array if no todos
	if len(todos) == 0 {
		return "[]", nil
	}

	// Format as JSON array
	data, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: failed to format todos: %v", err), err
	}

	return string(data), nil
}

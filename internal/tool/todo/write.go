package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/iav/internal/tool"
)

// WriteTodosTool handles writing todos.
type WriteTodosTool struct {
	store todoStore
}

// NewWriteTodosTool creates a new WriteTodosTool with injected dependencies.
func NewWriteTodosTool(store todoStore) *WriteTodosTool {
	if store == nil {
		panic("store is required")
	}
	return &WriteTodosTool{
		store: store,
	}
}

// Name returns the tool's identifier.
func (t *WriteTodosTool) Name() string {
	return "todowrite"
}

// Declaration returns the tool's schema for the LLM.
func (t *WriteTodosTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        "todowrite",
		Description: "Update the todo list. Replaces all todos with the provided list.",
		Parameters: &tool.Schema{
			Type: tool.TypeObject,
			Properties: map[string]*tool.Schema{
				"todos": {
					Type:        tool.TypeArray,
					Description: "List of todo items",
					Items: &tool.Schema{
						Type: tool.TypeObject,
						Properties: map[string]*tool.Schema{
							"description": {Type: tool.TypeString, Description: "Todo description"},
							"status":      {Type: tool.TypeString, Description: "Status: pending, in_progress, completed, cancelled"},
						},
						Required: []string{"description", "status"},
					},
				},
			},
			Required: []string{"todos"},
		},
	}
}

// WriteTodosRequest is the input for WriteTodosTool.
type WriteTodosRequest struct {
	Todos []Todo `json:"todos"`
}

// Prepare validates the request and returns an Invocation.
func (t *WriteTodosTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	req := &WriteTodosRequest{}
	if err := json.Unmarshal(params, req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate each todo
	for i, todo := range req.Todos {
		// Validate description is not empty
		if todo.Description == "" {
			return nil, fmt.Errorf("todo[%d]: description is required", i)
		}

		// Validate status
		switch todo.Status {
		case TodoStatusPending, TodoStatusInProgress, TodoStatusCompleted, TodoStatusCancelled:
			// Valid
		default:
			return nil, fmt.Errorf("todo[%d]: invalid status %q", i, todo.Status)
		}
	}

	return &writeTodosInvocation{
		store:   t.store,
		todos:   req.Todos,
		display: tool.StringDisplay(fmt.Sprintf("Writing %d todos", len(req.Todos))),
	}, nil
}

type writeTodosInvocation struct {
	store   todoStore
	todos   []Todo
	display tool.ToolDisplay
}

func (i *writeTodosInvocation) Display() tool.ToolDisplay {
	return i.display
}

func (i *writeTodosInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	if err := i.store.Write(i.todos); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to write todos: %v", err), err
	}

	// Return the updated todos as JSON array (per OpenCode format)
	if len(i.todos) == 0 {
		return "[]", nil
	}

	data, err := json.MarshalIndent(i.todos, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: failed to format todos: %v", err), err
	}

	return string(data), nil
}

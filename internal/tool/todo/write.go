package todo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
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

// Definition returns the tool's schema for the LLM using eino schema.
func (t *WriteTodosTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "todowrite",
		Desc: "Update the todo list. Replaces all todos with the provided list.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"todos": {
				Type: schema.Array,
				Desc: "List of todo items",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"description": {
							Type: schema.String,
							Desc: "Todo description",
						},
						"status": {
							Type: schema.String,
							Desc: "Status: pending, in_progress, completed, cancelled",
						},
					},
					Required: true,
				},
				Required: true,
			},
		}),
	}
}

// WriteTodosRequest is the input for WriteTodosTool.
type WriteTodosRequest struct {
	Todos []todo `json:"todos"`
}

// Prepare validates the request and returns an Invocation.
func (t *WriteTodosTool) Prepare(ctx context.Context, params string) (domain.Invocation, error) {
	req := &WriteTodosRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
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
		case todoStatusPending, todoStatusInProgress, todoStatusCompleted, todoStatusCancelled:
			// Valid
		default:
			return nil, fmt.Errorf("todo[%d]: invalid status %q", i, todo.Status)
		}
	}

	return &writeTodosInvocation{
		store:   t.store,
		todos:   req.Todos,
		display: domain.NewStringDisplay(fmt.Sprintf("Writing %d todos", len(req.Todos))),
	}, nil
}

type writeTodosInvocation struct {
	store   todoStore
	todos   []todo
	display domain.ToolDisplay
}

func (i *writeTodosInvocation) Display() domain.ToolDisplay {
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

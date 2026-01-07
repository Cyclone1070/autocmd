package todo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to execute write tool
func executeWrite(t *testing.T, tool *WriteTodosTool, req *WriteTodosRequest) (string, error) {
	t.Helper()
	params, _ := json.Marshal(req)
	inv, err := tool.Prepare(context.Background(), params)
	if err != nil {
		return "", err
	}
	return inv.Execute(context.Background())
}

func TestWriteTodosTool_Name(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	assert.Equal(t, "todowrite", tool.Name())
}

func TestWriteTodosTool_Declaration(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	decl := tool.Declaration()
	assert.Equal(t, "todowrite", decl.Name)
	assert.NotEmpty(t, decl.Description)
}

func TestWriteTodosTool_Success(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "Task 1", Status: TodoStatusPending},
			{Description: "Task 2", Status: TodoStatusInProgress},
		},
	}

	result, err := executeWrite(t, tool, req)
	require.NoError(t, err)

	// Verify JSON output
	var todos []Todo
	err = json.Unmarshal([]byte(result), &todos)
	require.NoError(t, err)
	assert.Len(t, todos, 2)

	// Verify store was updated
	assert.Len(t, store.todos, 2)
}

func TestWriteTodosTool_EmptyTodos(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{Todos: []Todo{}}
	result, err := executeWrite(t, tool, req)
	require.NoError(t, err)
	assert.Equal(t, "[]", result)
}

func TestWriteTodosTool_InvalidStatus(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "Task 1", Status: "invalid"},
		},
	}

	params, _ := json.Marshal(req)
	_, err := tool.Prepare(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestWriteTodosTool_EmptyDescription(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "", Status: TodoStatusPending},
		},
	}

	params, _ := json.Marshal(req)
	_, err := tool.Prepare(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "description is required")
}

func TestWriteTodosTool_StoreError(t *testing.T) {
	store := &mockTodoStore{writeErr: errors.New("store error")}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "Task 1", Status: TodoStatusPending},
		},
	}

	result, err := executeWrite(t, tool, req)
	require.Error(t, err)
	assert.Contains(t, result, "Error")
	assert.Contains(t, result, "store error")
}

func TestWriteTodosTool_AllStatuses(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "Pending", Status: TodoStatusPending},
			{Description: "In Progress", Status: TodoStatusInProgress},
			{Description: "Completed", Status: TodoStatusCompleted},
			{Description: "Cancelled", Status: TodoStatusCancelled},
		},
	}

	result, err := executeWrite(t, tool, req)
	require.NoError(t, err)

	var todos []Todo
	err = json.Unmarshal([]byte(result), &todos)
	require.NoError(t, err)
	assert.Len(t, todos, 4)
}

func TestWriteTodosTool_ContextCancelled(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewWriteTodosTool(store)

	req := &WriteTodosRequest{
		Todos: []Todo{
			{Description: "Task 1", Status: TodoStatusPending},
		},
	}

	params, _ := json.Marshal(req)
	inv, err := tool.Prepare(context.Background(), params)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := inv.Execute(ctx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Empty(t, result)
}

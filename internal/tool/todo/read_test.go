package todo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock store for testing
type mockTodoStore struct {
	todos    []Todo
	readErr  error
	writeErr error
}

func (m *mockTodoStore) Read() ([]Todo, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	result := make([]Todo, len(m.todos))
	copy(result, m.todos)
	return result, nil
}

func (m *mockTodoStore) Write(todos []Todo) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.todos = make([]Todo, len(todos))
	copy(m.todos, todos)
	return nil
}

// Helper to execute read tool
func executeRead(t *testing.T, tool *ReadTodosTool, params string) (string, error) {
	t.Helper()
	inv, err := tool.Prepare(context.Background(), json.RawMessage(params))
	if err != nil {
		return "", err
	}
	return inv.Execute(context.Background())
}

func TestReadTodosTool_Name(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewReadTodosTool(store)

	assert.Equal(t, "todoread", tool.Name())
}

func TestReadTodosTool_Declaration(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewReadTodosTool(store)

	decl := tool.Declaration()
	assert.Equal(t, "todoread", decl.Name)
	assert.NotEmpty(t, decl.Description)
}

func TestReadTodosTool_EmptyStore(t *testing.T) {
	store := &mockTodoStore{todos: []Todo{}}
	tool := NewReadTodosTool(store)

	result, err := executeRead(t, tool, "{}")
	require.NoError(t, err)
	assert.Equal(t, "[]", result)
}

func TestReadTodosTool_WithTodos(t *testing.T) {
	store := &mockTodoStore{
		todos: []Todo{
			{Description: "Task 1", Status: TodoStatusPending},
			{Description: "Task 2", Status: TodoStatusCompleted},
		},
	}
	tool := NewReadTodosTool(store)

	result, err := executeRead(t, tool, "{}")
	require.NoError(t, err)

	// Verify it's valid JSON
	var todos []Todo
	err = json.Unmarshal([]byte(result), &todos)
	require.NoError(t, err)
	assert.Len(t, todos, 2)
	assert.Equal(t, "Task 1", todos[0].Description)
	assert.Equal(t, TodoStatusPending, todos[0].Status)
}

func TestReadTodosTool_StoreError(t *testing.T) {
	store := &mockTodoStore{readErr: errors.New("store error")}
	tool := NewReadTodosTool(store)

	result, err := executeRead(t, tool, "{}")
	require.Error(t, err)
	assert.Contains(t, result, "Error")
	assert.Contains(t, result, "store error")
}

func TestReadTodosTool_ContextCancelled(t *testing.T) {
	store := &mockTodoStore{}
	tool := NewReadTodosTool(store)

	inv, err := tool.Prepare(context.Background(), json.RawMessage("{}"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := inv.Execute(ctx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Empty(t, result)
}

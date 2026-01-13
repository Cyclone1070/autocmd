package todo

import (
	"sync"
)

// InMemoryTodoStore implements todo storage using an in-memory slice.
type InMemoryTodoStore struct {
	todos []todo
	mu    sync.RWMutex
}

// NewInMemoryTodoStore creates a new instance of InMemoryTodoStore.
func NewInMemoryTodoStore() *InMemoryTodoStore {
	return &InMemoryTodoStore{
		todos: make([]todo, 0),
	}
}

// Read returns the current list of todos.
func (s *InMemoryTodoStore) Read() ([]todo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to ensure isolation
	result := make([]todo, len(s.todos))
	copy(result, s.todos)
	return result, nil
}

// Write replaces the current list of todos with the provided list.
func (s *InMemoryTodoStore) Write(todos []todo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy to ensure isolation
	s.todos = make([]todo, len(todos))
	copy(s.todos, todos)
	return nil
}

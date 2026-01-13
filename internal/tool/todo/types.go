package todo

// todoStatus represents the status of a todo item.
type todoStatus string

const (
	todoStatusPending    todoStatus = "pending"
	todoStatusInProgress todoStatus = "in_progress"
	todoStatusCompleted  todoStatus = "completed"
	todoStatusCancelled  todoStatus = "cancelled"
)

// todo represents a single todo item.
type todo struct {
	Description string     `json:"description"`
	Status      todoStatus `json:"status"`
}

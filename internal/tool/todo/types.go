package todo

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusCancelled  TodoStatus = "cancelled"
)

// Todo represents a single todo item.
type Todo struct {
	Description string     `json:"description"`
	Status      TodoStatus `json:"status"`
}

package todo

// todoStore defines the interface for todo storage.
// This is a consumer-defined interface per architecture guidelines.
type todoStore interface {
	Read() ([]Todo, error)
	Write(todos []Todo) error
}

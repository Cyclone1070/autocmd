package todo

// todoStore defines the interface for todo storage.
// This is a consumer-defined interface per architecture guidelines.
type todoStore interface {
	Read() ([]todo, error)
	Write(todos []todo) error
}

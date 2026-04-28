// Package domain defines the core domain models and interfaces for the IAV system.
package domain

// User actions: Action marker and StopAction.
// Feature-specific actions live next to their types (e.g. question tool in tool_types.go).

// Action is the interface for all intents flowing from UI to Workflow.
type Action interface {
	isAction()
}

// CallIDer is implemented by actions correlated to a tool call (e.g. question answers).
type CallIDer interface {
	Action
	GetCallID() string
}

// StopAction is a user intent to cancel the current workflow.
type StopAction struct{}

func (StopAction) isAction() {}

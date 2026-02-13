package ui

// ToolStatus represents tool lifecycle state for display/theme rendering.
type ToolStatus int

const (
	StatusRunning ToolStatus = iota
	StatusSuccess
	StatusError
)

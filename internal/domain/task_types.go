package domain

// TaskResult represents the result of a completed background task.
type TaskResult struct {
	ID          string
	Status      string // "success", "failed", "stopped"
	Description string
	Command     string
	ExitCode    int
	Error       string
	LogPath     string
}

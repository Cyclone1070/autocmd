package domain

// TaskResult represents the result of a completed background task.
type TaskResult struct {
	ID          string `xml:"task-id"`
	Status      string `xml:"status"`
	Description string `xml:"description"`
	Command     string `xml:"command"`
	ExitCode    int    `xml:"exit-code"`
	Error       string `xml:"error,omitempty"`
	LogPath     string `xml:"log-file"`
	Cwd         string `xml:"cwd"`
}

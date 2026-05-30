package domain

// State holds application persistent state loaded from disk.
type State struct {
	Model string `json:"model"`
}

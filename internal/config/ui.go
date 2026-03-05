package config

// ColorConfig holds light and dark hex codes for a specific semantic role.
type ColorConfig struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// UIConfig holds styling configuration for the terminal UI.
type UIConfig struct {
	PrimaryColor      ColorConfig `json:"primary_color"` // Default: Sky Blue
	SuccessColor      ColorConfig `json:"success_color"` // Default: Green
	ErrorColor        ColorConfig `json:"error_color"`   // Default: Red
	MutedColor        ColorConfig `json:"muted_color"`   // Default: Gray (borders, metadata)
	ChatWindowWidth   int         `json:"chat_window_width"`
	ShellOutputHeight int         `json:"shell_output_height"` // Visible lines in shell tool output (Default: 12)
}

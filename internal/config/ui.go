package config

// ColorConfig holds light and dark hex codes for a specific semantic role.
type ColorConfig struct {
	light string
	dark  string
}

func (c ColorConfig) Light() string { return c.light }
func (c ColorConfig) Dark() string  { return c.dark }

// UIConfig holds styling configuration for the terminal UI.
type UIConfig struct {
	primaryColor     ColorConfig
	successColor     ColorConfig
	errorColor       ColorConfig
	mutedColor       ColorConfig
	chatWindowWidth  int
	bashOutputHeight int
	shortToolBlock   bool
}

func (c UIConfig) PrimaryColor() ColorConfig { return c.primaryColor }
func (c UIConfig) SuccessColor() ColorConfig { return c.successColor }
func (c UIConfig) ErrorColor() ColorConfig   { return c.errorColor }
func (c UIConfig) MutedColor() ColorConfig   { return c.mutedColor }
func (c UIConfig) ChatWindowWidth() int      { return c.chatWindowWidth }
func (c UIConfig) BashOutputHeight() int     { return c.bashOutputHeight }
func (c UIConfig) ShortToolBlock() bool      { return c.shortToolBlock }

// Setters for testing

func (c *UIConfig) SetChatWindowWidth(w int) { c.chatWindowWidth = w }
func (c *UIConfig) SetShortToolBlock(b bool) { c.shortToolBlock = b }

// DTOs for JSON persistence.
type colorDTO struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

type uiDTO struct {
	PrimaryColor     colorDTO `json:"primary_color"`
	SuccessColor     colorDTO `json:"success_color"`
	ErrorColor       colorDTO `json:"error_color"`
	MutedColor       colorDTO `json:"muted_color"`
	ChatWindowWidth  int      `json:"chat_window_width"`
	BashOutputHeight int      `json:"bash_output_height"`
	ShortToolBlock   bool     `json:"short_tool_block"`
}

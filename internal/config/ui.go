package config

// ColorConfig holds light and dark hex codes for a specific semantic role.
type ColorConfig struct {
	light string
	dark  string
}

// Light returns the hex code for light mode.
func (c ColorConfig) Light() string { return c.light }

// Dark returns the hex code for dark mode.
func (c ColorConfig) Dark() string { return c.dark }

// UIConfig holds styling configuration for the terminal UI.
type UIConfig struct {
	primaryColor     ColorConfig
	successColor     ColorConfig
	errorColor       ColorConfig
	mutedColor       ColorConfig
	textColor        ColorConfig
	chatWindowWidth  int
	bashOutputHeight int
	thinkingHeight   int
	shortToolBlock   bool
}

// PrimaryColor returns the primary brand color.
func (c UIConfig) PrimaryColor() ColorConfig { return c.primaryColor }

// SuccessColor returns the color used for success indicators.
func (c UIConfig) SuccessColor() ColorConfig { return c.successColor }

// ErrorColor returns the color used for error indicators.
func (c UIConfig) ErrorColor() ColorConfig { return c.errorColor }

// MutedColor returns the color used for secondary or less important text.
func (c UIConfig) MutedColor() ColorConfig { return c.mutedColor }

// TextColor returns the color used for primary body text.
func (c UIConfig) TextColor() ColorConfig { return c.textColor }

// ChatWindowWidth returns the width of the chat UI in characters.
func (c UIConfig) ChatWindowWidth() int { return c.chatWindowWidth }

// BashOutputHeight returns the maximum height of bash output blocks.
func (c UIConfig) BashOutputHeight() int { return c.bashOutputHeight }

// ThinkingHeight returns the height of the thinking/process indicator.
func (c UIConfig) ThinkingHeight() int { return c.thinkingHeight }

// ShortToolBlock returns whether tool outputs should be rendered in a compact format.
func (c UIConfig) ShortToolBlock() bool { return c.shortToolBlock }

// Setters for testing

// SetChatWindowWidth updates the chat window width.
func (c *UIConfig) SetChatWindowWidth(w int) { c.chatWindowWidth = w }

// SetShortToolBlock updates the short tool block setting.
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
	TextColor        colorDTO `json:"text_color"`
	ChatWindowWidth  int      `json:"chat_window_width"`
	BashOutputHeight int      `json:"bash_output_height"`
	ThinkingHeight   int      `json:"thinking_height"`
	ShortToolBlock   bool     `json:"short_tool_block"`
}

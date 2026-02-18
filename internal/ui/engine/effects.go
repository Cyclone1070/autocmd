package engine

// Effect represents a side effect the engine requests.
// The runtime (Bubble Tea adapter) interprets these into tea.Cmd.
type Effect interface {
	isEffect()
	IsTick() bool
	IsQuit() bool
	IsPrint() bool
}

// PrintPayload is the concrete type for print effects (exported for interpreter).
type PrintPayload struct {
	Content string
	Raw     bool
}

func (PrintPayload) isEffect()     {}
func (PrintPayload) IsTick() bool  { return false }
func (PrintPayload) IsQuit() bool  { return false }
func (PrintPayload) IsPrint() bool { return true }

// EffectPrint creates a print effect (tea.Println semantics).
func EffectPrint(content string) Effect {
	return PrintPayload{Content: content, Raw: false}
}

// EffectPrintRaw creates a raw print effect (tea.Printf semantics).
func EffectPrintRaw(content string) Effect {
	return PrintPayload{Content: content, Raw: true}
}

type effectScheduleTick struct{}

func (effectScheduleTick) isEffect()             {}
func (effectScheduleTick) isEffectScheduleTick() {}
func (effectScheduleTick) IsTick() bool          { return true }
func (effectScheduleTick) IsQuit() bool          { return false }
func (effectScheduleTick) IsPrint() bool         { return false }

// EffectScheduleTick requests the next simulation tick.
func EffectScheduleTick() Effect {
	return effectScheduleTick{}
}

// QuitPayload is the concrete type for quit effect (exported for interpreter).
type QuitPayload struct{}

func (QuitPayload) isEffect()     {}
func (QuitPayload) IsTick() bool  { return false }
func (QuitPayload) IsQuit() bool  { return true }
func (QuitPayload) IsPrint() bool { return false }

// EffectQuit requests program quit.
func EffectQuit() Effect {
	return QuitPayload{}
}

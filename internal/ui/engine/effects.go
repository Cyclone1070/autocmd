package engine

// Effect represents a side effect the engine requests.
// The runtime (Bubble Tea adapter) interprets these into tea.Cmd.
type Effect interface {
	isEffect()
}

// PrintPayload is the concrete type for print effects (exported for interpreter).
type PrintPayload struct {
	Content string
	Raw     bool
}

func (PrintPayload) isEffect() {}

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

// EffectScheduleTick requests the next simulation tick.
func EffectScheduleTick() Effect {
	return effectScheduleTick{}
}

// QuitPayload is the concrete type for quit effect (exported for interpreter).
type QuitPayload struct{}

func (QuitPayload) isEffect() {}

// EffectQuit requests program quit.
func EffectQuit() Effect {
	return QuitPayload{}
}

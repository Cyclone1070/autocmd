package tea

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/ui/engine"
)

func TestInterpret_Print_HandledByAdapter(t *testing.T) {
	eff := engine.EffectPrint("hello")
	cmd := Interpret(eff)
	// PrintPayload is handled by adapter via FrameSink, not Interpret
	if cmd != nil {
		t.Error("Interpret must return nil for PrintPayload (adapter handles via sink)")
	}
}

func TestInterpret_PrintRaw_HandledByAdapter(t *testing.T) {
	eff := engine.EffectPrintRaw("raw")
	cmd := Interpret(eff)
	if cmd != nil {
		t.Error("Interpret must return nil for PrintPayload (adapter handles via sink)")
	}
}

func TestInterpret_Quit_HandledByAdapter(t *testing.T) {
	eff := engine.EffectQuit()
	cmd := Interpret(eff)
	if cmd != nil {
		t.Error("Interpret must return nil for QuitPayload (adapter handles via sink)")
	}
}

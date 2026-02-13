package runtime

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/ui/engine"
)

func TestInterpret_Print(t *testing.T) {
	eff := engine.EffectPrint("hello")
	cmd := Interpret(eff)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for EffectPrint")
	}
	// Run the cmd - tea.Sequence may produce internal msgs; we just verify it runs
	_ = cmd()
}

func TestInterpret_PrintRaw(t *testing.T) {
	eff := engine.EffectPrintRaw("raw")
	cmd := Interpret(eff)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for EffectPrintRaw")
	}
	_ = cmd()
}

func TestInterpret_Quit(t *testing.T) {
	eff := engine.EffectQuit()
	cmd := Interpret(eff)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for EffectQuit")
	}
	_ = cmd
}

package tea

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
)

func TestNewTeaModelAdapter_PanicsOnNilSink(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when sink is nil")
		}
	}()
	geom := engine.TermSize{Width: 80, Height: 24}
	state := engine.NewInitialState(geom)
	factory := func() engine.Deps { return engine.Deps{} }
	_ = NewTeaModelAdapter(state, factory, nil)
}

func TestToEngineMsg_DomainEvents(t *testing.T) {
	tests := []struct {
		name   string
		teaMsg interface{}
		want   bool
	}{
		{"Tick", engine.MsgTick{}, true},
		{"Text", domain.TextEvent{Text: "hi"}, true},
		{"Done", domain.DoneEvent{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := toEngineMsg(tt.teaMsg)
			if ok != tt.want {
				t.Errorf("toEngineMsg() ok = %v, want %v", ok, tt.want)
			}
		})
	}
}

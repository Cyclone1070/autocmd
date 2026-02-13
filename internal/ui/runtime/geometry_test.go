package runtime

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
)

type staticCursorDetector struct {
	row int
}

func (d *staticCursorDetector) GetCursorRow() (int, error) {
	return d.row, nil
}

func TestResolveGeometry(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = 80
	cd := &staticCursorDetector{row: 5}

	geom, err := ResolveGeometry(cfg, cd, 24)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}

	if geom.Width != 80 {
		t.Errorf("Width = %d, want 80", geom.Width)
	}
	if geom.TermHeight != 24 {
		t.Errorf("TermHeight = %d, want 24", geom.TermHeight)
	}
	// spaceBelow = 24 - 5 - 2 = 17
	if geom.SpaceBelow != 17 {
		t.Errorf("SpaceBelow = %d, want 17", geom.SpaceBelow)
	}
}

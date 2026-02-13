package runtime

import (
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/ui/engine"
)

// CursorDetector returns the current 1-based cursor row.
type CursorDetector interface {
	GetCursorRow() (int, error)
}

const statusBarOverhead = 2

// ResolveGeometry computes engine.Geometry from config and cursor position.
// termHeight can be 0 to use default 24.
func ResolveGeometry(cfg *config.Config, cd CursorDetector, termHeight int) (engine.Geometry, error) {
	width := cfg.UI.ChatWindowWidth
	height := termHeight
	if height <= 0 {
		height = 24
	}

	row, err := cd.GetCursorRow()
	if err != nil {
		row = 1
	}
	spaceBelow := max(height-row-statusBarOverhead, 0)

	return engine.Geometry{
		Width:      width,
		TermHeight: height,
		SpaceBelow: spaceBelow,
	}, nil
}

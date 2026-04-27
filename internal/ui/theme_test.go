package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestTheme_RenderToolBlock_HeaderAndGutter(t *testing.T) {
	cfg := config.DefaultConfig().UI()
	th := NewTheme(ThemeConfig{
		PrimaryColor:   ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	})
	spec := ToolBlockSpec{
		HeaderLines:  []string{"Read \"main.go\""},
		ContentLines: []string{"first line", "second line"},
		Status:       StatusSuccess,
	}

	got := th.RenderToolBlock(spec)

	assert.Equal(t, "\n\n    ✔ Read \"main.go\"\n       ⎿ first line\n         second line", got)
}

func TestTheme_RenderToolBlock_HeaderContinuationIndented(t *testing.T) {
	cfg := config.DefaultConfig().UI()
	th := NewTheme(ThemeConfig{
		PrimaryColor:   ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessColor:   ToAdaptiveColor(cfg.SuccessColor()),
		ErrorColor:     ToAdaptiveColor(cfg.ErrorColor()),
		MutedColor:     ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	})
	spec := ToolBlockSpec{
		HeaderLines: []string{"Read \"main.go\"", "with extra context"},
		Status:      StatusSuccess,
	}

	got := th.RenderToolBlock(spec)

	assert.Equal(t, "\n\n    ✔ Read \"main.go\"\n      with extra context", got)
}

package ui

import (
	"testing"

	"github.com/Cyclone1070/autocmd/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestTheme_RenderActionBlock_HeaderAndGutter(t *testing.T) {
	cfg := config.DefaultConfig().UI()
	th := &Theme{
		PrimaryCol:     ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessCol:     ToAdaptiveColor(cfg.SuccessColor()),
		ErrorCol:       ToAdaptiveColor(cfg.ErrorColor()),
		MutedCol:       ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	spec := ActionBlockSpec{
		HeaderLines:  []string{"Read \"main.go\""},
		ContentLines: []string{"first line", "second line"},
		Status:       StatusSuccess,
	}

	got := th.RenderActionBlock(spec)

	assert.Equal(t, "\n\n    ✔ Read \"main.go\"\n       ⎿ first line\n         second line", got)
}

func TestTheme_RenderActionBlock_HeaderContinuationIndented(t *testing.T) {
	cfg := config.DefaultConfig().UI()
	th := &Theme{
		PrimaryCol:     ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessCol:     ToAdaptiveColor(cfg.SuccessColor()),
		ErrorCol:       ToAdaptiveColor(cfg.ErrorColor()),
		MutedCol:       ToAdaptiveColor(cfg.MutedColor()),
		ShortToolBlock: cfg.ShortToolBlock(),
	}
	spec := ActionBlockSpec{
		HeaderLines: []string{"Read \"main.go\"", "with extra context"},
		Status:      StatusSuccess,
	}

	got := th.RenderActionBlock(spec)

	assert.Equal(t, "\n\n    ✔ Read \"main.go\"\n      with extra context", got)
}

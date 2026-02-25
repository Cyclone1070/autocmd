package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func runModelPicker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	registry, err := buildLLMRegistry(ctx, cfg)
	if err != nil {
		return err
	}

	models, err := registry.List(ctx)
	if err != nil {
		return err
	}

	var items []ui.Item
	for _, m := range models {
		parts := strings.SplitN(m.ID, "/", 2)
		provider := "unknown"
		if len(parts) == 2 {
			provider = parts[0]
		}

		items = append(items, ui.Item{
			ID:     m.ID,
			Label:  m.DisplayName,
			Detail: m.ID,
			Active: m.ID == cfg.Model,
			Group:  provider,
		})
	}

	pickerCfg := ui.Config{
		Title: "MODELS",
		Items: items,
	}

	m := ui.NewPicker(pickerCfg)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("picker failed: %w", err)
	}

	if selected, ok := m.Selected(); ok {
		cfg.Model = selected.ID
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("\nSelected model: %s\n", selected.ID)
	}

	return nil
}

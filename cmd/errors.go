package cmd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/Cyclone1070/iav/internal/agent"
)

var (
	ErrBootstrap            = errors.New("bootstrap failure")
	ErrWorkspaceUnavailable = errors.New("workspace unavailable")
	ErrModelInitialization  = errors.New("model initialization failure")
	ErrModelBackend         = errors.New("model backend failure")
	ErrModelAuth            = errors.New("model authentication failure")
	ErrUIRuntime            = errors.New("ui runtime failure")
	ErrNoModelSelected      = errors.New("no model selected")
)

func wrapForUser(err error) error {
	if err == nil {
		return nil
	}
	slog.Error("command failed", "error", err)
	return errors.New(mapUserFacingError(err))
}

func mapUserFacingError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrNoModelSelected):
		return "No model selected. Please run 'iav model' or 'iav auth' to get started"
	case errors.Is(err, ErrBootstrap):
		return "Failed to load app configuration/state. Check config and retry."
	case errors.Is(err, ErrWorkspaceUnavailable):
		return "Could not access current workspace. Verify directory exists and permissions."
	case errors.Is(err, ErrModelInitialization):
		return "Could not initialize selected model/provider. Run 'iav auth' or 'iav model'."
	case errors.Is(err, ErrModelAuth), errors.Is(err, agent.ErrModelAuth):
		return "Authentication failed for the selected model (invalid/expired API key or token). Run 'iav auth' and try again."
	case errors.Is(err, ErrModelBackend), errors.Is(err, agent.ErrModelBackend):
		return "Model backend error. Retry, or switch model with 'iav model'."
	case errors.Is(err, ErrUIRuntime):
		return "Terminal UI failed to start. Retry in a standard terminal session."
	default:
		return "Unexpected internal error. See log file for details."
	}
}

func withCategory(category error, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", category, err)
}

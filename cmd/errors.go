package cmd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/Cyclone1070/iav/internal/agent"
)

var (
	errBootstrap            = errors.New("bootstrap failure")
	errWorkspaceUnavailable = errors.New("workspace unavailable")
	errModelInitialization  = errors.New("model initialization failure")
	errModelBackend         = errors.New("model backend failure")
	errModelAuth            = errors.New("model authentication failure")
	errUIRuntime            = errors.New("ui runtime failure")
	errNoModelSelected      = errors.New("no model selected")
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
	case errors.Is(err, errNoModelSelected):
		return "No model selected. Please run 'iav model' or 'iav auth' to get started"
	case errors.Is(err, errBootstrap):
		return "Failed to load app configuration/state. Check config and retry."
	case errors.Is(err, errWorkspaceUnavailable):
		return "Could not access current workspace. Verify directory exists and permissions."
	case errors.Is(err, errModelInitialization):
		return "Could not initialize selected model/provider. Run 'iav auth' or 'iav model'."
	case errors.Is(err, errModelAuth), errors.Is(err, agent.ErrModelAuth):
		return "Authentication failed for the selected model (invalid/expired API key or token). Run 'iav auth' and try again."
	case errors.Is(err, errModelBackend), errors.Is(err, agent.ErrModelBackend):
		return "Model backend error. Retry, or switch model with 'iav model'."
	case errors.Is(err, errUIRuntime):
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

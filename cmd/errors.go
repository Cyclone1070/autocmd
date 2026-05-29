package cmd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/Cyclone1070/autocmd/internal/agent"
)

var (
	errModelProvider        = errors.New("model provider failure")
	errAgenticLoop          = errors.New("agentic loop failure")
	errWorkspaceUnavailable = errors.New("workspace unavailable")
	errSetup                = errors.New("setup failure")
)

//nolint:staticcheck // We capitalize categories for user-facing CLI presentation
func wrapForUser(err error) error {
	if err == nil {
		return nil
	}
	slog.Error("command failed", "error", err)

	root := rootError(err)

	switch {
	case errors.Is(err, errModelProvider), errors.Is(err, agent.ErrModel):
		return fmt.Errorf("LLM Provider Failed: %w", root)

	case errors.Is(err, errAgenticLoop):
		return fmt.Errorf("Agentic Loop Failed: %w", root)

	case errors.Is(err, errWorkspaceUnavailable):
		return fmt.Errorf("Workspace Access Failed: %w", root)

	case errors.Is(err, errSetup):
		return fmt.Errorf("Setup Failed: %w", root)

	default:
		return root
	}
}

func rootError(err error) error {
	for {
		//nolint:errorlint // We explicitly inspect the current error wrapper in the chain
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			u := x.Unwrap()
			if u == nil {
				return err
			}
			err = u
		case interface{ Unwrap() []error }:
			u := x.Unwrap()
			if len(u) == 0 {
				return err
			}
			err = u[len(u)-1]
		default:
			return err
		}
	}
}

func withCategory(category error, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", category, err)
}

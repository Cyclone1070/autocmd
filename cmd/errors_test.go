package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Cyclone1070/iav/internal/agent"
	"github.com/stretchr/testify/assert"
)

func TestMapUserFacingError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "provider internal stream error",
			err:  fmt.Errorf("%w: %w", ErrModelBackend, agent.ErrModelBackend),
			want: "Model backend error. Retry, or switch model with 'iav model'.",
		},
		{
			name: "provider auth error",
			err:  fmt.Errorf("%w: %w", ErrModelAuth, agent.ErrModelAuth),
			want: "Authentication failed for the selected model (invalid/expired API key or token). Run 'iav auth' and try again.",
		},
		{
			name: "workspace root issue",
			err:  fmt.Errorf("%w: invalid workspace root", ErrWorkspaceUnavailable),
			want: "Could not access current workspace. Verify directory exists and permissions.",
		},
		{
			name: "config issue",
			err:  fmt.Errorf("%w: parse failure", ErrBootstrap),
			want: "Failed to load app configuration/state. Check config and retry.",
		},
		{
			name: "auth model init issue",
			err:  fmt.Errorf("%w: credential missing", ErrModelInitialization),
			want: "Could not initialize selected model/provider. Run 'iav auth' or 'iav model'.",
		},
		{
			name: "ui startup issue",
			err:  fmt.Errorf("%w: bad tty", ErrUIRuntime),
			want: "Terminal UI failed to start. Retry in a standard terminal session.",
		},
		{
			name: "no model selected",
			err:  ErrNoModelSelected,
			want: "No model selected. Please run 'iav model' or 'iav auth' to get started",
		},
		{
			name: "fallback",
			err:  errors.New("something unexpected exploded"),
			want: "Unexpected internal error. See log file for details.",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, mapUserFacingError(tc.err))
		})
	}
}


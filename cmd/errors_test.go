package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/agent"
	"github.com/stretchr/testify/assert"
)

func TestWrapForUser(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "LLM Provider Failure (Backend)",
			err:  withCategory(errModelProvider, fmt.Errorf("LLM.Stream: %w", errors.New("error 500: Internal Server Error"))),
			want: "LLM Provider Failed: error 500: Internal Server Error",
		},
		{
			name: "LLM Provider Failure (Auth)",
			err:  withCategory(errModelProvider, agent.ErrModel),
			want: "LLM Provider Failed: model failure",
		},
		{
			name: "Agentic Loop Failure (Max Iterations)",
			err:  withCategory(errAgenticLoop, errors.New("max iterations (10) reached")),
			want: "Agentic Loop Failed: max iterations (10) reached",
		},
		{
			name: "Workspace Access Failure",
			err:  withCategory(errWorkspaceUnavailable, errors.New("stat /wrong/path: no such file or directory")),
			want: "Workspace Access Failed: stat /wrong/path: no such file or directory",
		},
		{
			name: "Setup Failure (Bootstrap)",
			err:  withCategory(errSetup, errors.New("invalid config yaml")),
			want: "Setup Failed: invalid config yaml",
		},
		{
			name: "Setup Failure (Model Init)",
			err:  withCategory(errSetup, errors.New("LLM provider unknown")),
			want: "Setup Failed: LLM provider unknown",
		},
		{
			name: "Setup Failure (UI Runtime)",
			err:  withCategory(errSetup, errors.New("terminal size too small")),
			want: "Setup Failed: terminal size too small",
		},
		{
			name: "Setup Failure (No Model Selected)",
			err:  withCategory(errSetup, errors.New("no model selected")),
			want: "Setup Failed: no model selected",
		},
		{
			name: "Fallback raw error without categories",
			err:  errors.New("raw unexpected failure"),
			want: "raw unexpected failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := wrapForUser(tc.err)
			if tc.want == "" {
				assert.NoError(t, got)
			} else {
				assert.Error(t, got)
				assert.Equal(t, tc.want, got.Error())
			}
		})
	}
}

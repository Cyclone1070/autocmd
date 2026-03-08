package cmd

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAgent_FailFast_NoModel(t *testing.T) {
	cfg := config.DefaultConfig()
	s := &state.State{Model: ""}
	ctx := context.Background()

	err := runAgent(ctx, cfg, s, "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "No model selected")
	assert.Contains(t, err.Error(), "Please run 'iav model' or 'iav auth'")
}

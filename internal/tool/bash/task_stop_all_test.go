package bash

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestTaskStopAllTool_Execute_Success(t *testing.T) {
	ctx := context.Background()

	mockTM := new(mockTaskManager)
	tl := NewTaskStopAllTool(mockTM)

	mockTM.On("StopAll")

	resLLM, disp := tl.executeTaskStopAll(ctx, nil)

	assert.Equal(t, "all background tasks stopped", resLLM)
	assert.Equal(t, "Stop all background bash tasks", disp.(domain.StringDisplay).Description)
	mockTM.AssertExpectations(t)
}

func TestTaskStopAllTool_Info_HasEmptyParams(t *testing.T) {
	tl := NewTaskStopAllTool(nil)
	info, err := tl.Info(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, info.ParamsOneOf, "Expected ParamsOneOf to be set")
}

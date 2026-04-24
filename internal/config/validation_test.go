package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_AllDefaults_Pass(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_Tools(t *testing.T) {
	t.Run("Zero File Size Fails", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.tools.maxFileSize = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_file_size")
	})

	t.Run("Zero Iterations Fails", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.tools.maxIterations = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_iterations")
	})
}

func TestValidate_MultipleErrors_ReportsAll(t *testing.T) {
	cfg := DefaultConfig()
	cfg.tools.maxFileSize = 0
	cfg.tools.maxIterations = 0

	err := cfg.Validate()
	assert.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "max_file_size")
	assert.Contains(t, msg, "max_iterations")
}

func TestValidate_ToolPermissions(t *testing.T) {
	t.Run("Invalid Default Fails", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.tools.permissionDefault = "sometimes"
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tools.permissions.default")
	})

	t.Run("Invalid ByTool Fails", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.tools.toolPermissions["bash"] = "sometimes"
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tools.permissions.by_tool.bash")
	})
}

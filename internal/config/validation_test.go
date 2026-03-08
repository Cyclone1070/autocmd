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
		cfg.Tools.MaxFileSize = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_file_size")
	})

	t.Run("Zero Iterations Fails", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Tools.MaxIterations = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max_iterations")
	})
}

func TestValidate_MultipleErrors_ReportsAll(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tools.MaxFileSize = 0
	cfg.Tools.DefaultShellTimeout = 0

	err := cfg.Validate()
	assert.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "max_file_size")
	assert.Contains(t, msg, "default_shell_timeout")
}

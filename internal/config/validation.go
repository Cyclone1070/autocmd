package config

import (
	"fmt"
	"regexp"
)

// Validate checks config values for life correctness.
// Returns an error if any values are invalid.
func (c *Config) Validate() error {
	var errs []string

	// Tools validation
	if c.tools.maxFileSize < 1 {
		errs = append(errs, "tools.max_file_size must be >= 1")
	}
	if c.tools.defaultShellTimeout < 1 {
		errs = append(errs, "tools.default_shell_timeout must be >= 1")
	}
	if c.tools.maxIterations < 1 {
		errs = append(errs, "tools.max_iterations must be >= 1")
	}

	// Session validation
	if c.session.storageDir == "" {
		errs = append(errs, "session.storage_dir must not be empty")
	}

	// UI validation
	hexRegex := regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
	validateColor := func(path string, col ColorConfig) {
		if !hexRegex.MatchString(col.light) {
			errs = append(errs, fmt.Sprintf("ui.%s.light must be a valid hex color", path))
		}
		if !hexRegex.MatchString(col.dark) {
			errs = append(errs, fmt.Sprintf("ui.%s.dark must be a valid hex color", path))
		}
	}

	validateColor("primary_color", c.ui.primaryColor)
	validateColor("success_color", c.ui.successColor)
	validateColor("error_color", c.ui.errorColor)
	validateColor("muted_color", c.ui.mutedColor)

	if c.ui.chatWindowWidth < 1 {
		errs = append(errs, "ui.chat_window_width must be >= 1")
	}
	if c.ui.shellOutputHeight < 1 {
		errs = append(errs, "ui.shell_output_height must be >= 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %v", errs)
	}

	return nil
}

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
	if c.Tools.MaxFileSize < 1 {
		errs = append(errs, "tools.max_file_size must be >= 1")
	}
	if c.Tools.DefaultShellTimeout < 1 {
		errs = append(errs, "tools.default_shell_timeout must be >= 1")
	}
	if c.Tools.MaxIterations < 1 {
		errs = append(errs, "tools.max_iterations must be >= 1")
	}

	// Session validation
	if c.Session.StorageDir == "" {
		errs = append(errs, "session.storage_dir must not be empty")
	}

	// UI validation
	hexRegex := regexp.MustCompile(`^#([A-Fa-f0-9]{6}|[A-Fa-f0-9]{3})$`)
	validateColor := func(path string, col ColorConfig) {
		if !hexRegex.MatchString(col.Light) {
			errs = append(errs, fmt.Sprintf("ui.%s.light must be a valid hex color", path))
		}
		if !hexRegex.MatchString(col.Dark) {
			errs = append(errs, fmt.Sprintf("ui.%s.dark must be a valid hex color", path))
		}
	}

	validateColor("primary_color", c.UI.PrimaryColor)
	validateColor("success_color", c.UI.SuccessColor)
	validateColor("error_color", c.UI.ErrorColor)
	validateColor("muted_color", c.UI.MutedColor)

	if c.UI.ChatWindowWidth < 1 {
		errs = append(errs, "ui.chat_window_width must be >= 1")
	}
	if c.UI.ShellOutputHeight < 1 {
		errs = append(errs, "ui.shell_output_height must be >= 1")
	}


	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %v", errs)
	}

	return nil
}

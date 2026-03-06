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
	if c.Tools.DefaultReadFileLimit < 1 {
		errs = append(errs, "tools.default_read_file_limit must be >= 1")
	}
	if c.Tools.DefaultListDirectoryLimit < 1 {
		errs = append(errs, "tools.default_list_directory_limit must be >= 1")
	}
	if c.Tools.MaxListDirectoryLimit < 1 {
		errs = append(errs, "tools.max_list_directory_limit must be >= 1")
	}
	if c.Tools.MaxListDirectoryResults < 1 {
		errs = append(errs, "tools.max_list_directory_results must be >= 1")
	}
	if c.Tools.DefaultMaxCommandOutputSize < 1 {
		errs = append(errs, "tools.default_max_command_output_size must be >= 1")
	}
	if c.Tools.DefaultShellTimeout < 1 {
		errs = append(errs, "tools.default_shell_timeout must be >= 1")
	}

	// Tools validation - Search & Find
	if c.Tools.MaxLineLength < 1 {
		errs = append(errs, "tools.max_line_length must be >= 1")
	}
	if c.Tools.MaxSearchContentResults < 1 {
		errs = append(errs, "tools.max_search_content_results must be >= 1")
	}
	if c.Tools.DefaultSearchContentLimit < 1 {
		errs = append(errs, "tools.default_search_content_limit must be >= 1")
	}
	if c.Tools.MaxSearchContentLimit < 1 {
		errs = append(errs, "tools.max_search_content_limit must be >= 1")
	}
	if c.Tools.MaxFindFileResults < 1 {
		errs = append(errs, "tools.max_find_file_results must be >= 1")
	}
	if c.Tools.DefaultFindFileLimit < 1 {
		errs = append(errs, "tools.default_find_file_limit must be >= 1")
	}
	if c.Tools.MaxFindFileLimit < 1 {
		errs = append(errs, "tools.max_find_file_limit must be >= 1")
	}

	// Semantic validation: Default <= Max constraints
	if c.Tools.DefaultListDirectoryLimit > c.Tools.MaxListDirectoryLimit {
		errs = append(errs, "tools.default_list_directory_limit must be <= tools.max_list_directory_limit")
	}
	if c.Tools.DefaultSearchContentLimit > c.Tools.MaxSearchContentLimit {
		errs = append(errs, "tools.default_search_content_limit must be <= tools.max_search_content_limit")
	}
	if c.Tools.DefaultFindFileLimit > c.Tools.MaxFindFileLimit {
		errs = append(errs, "tools.default_find_file_limit must be <= tools.max_find_file_limit")
	}

	// Tools validation - Docker
	if c.Tools.DockerRetryAttempts < 1 {
		errs = append(errs, "tools.docker_retry_attempts must be >= 1")
	}
	if c.Tools.DockerRetryIntervalMs < 1 {
		errs = append(errs, "tools.docker_retry_interval_ms must be >= 1")
	}
	if c.Tools.DockerGracefulShutdownMs < 1 {
		errs = append(errs, "tools.docker_graceful_shutdown_ms must be >= 1")
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

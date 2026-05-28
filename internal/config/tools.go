package config

import "maps"

// ToolsConfig holds tool-specific configuration.
type ToolsConfig struct {
	toolPermissions   map[string]string
	permissionDefault string
	maxFileSize       int64
	maxIterations     int
}

// MaxFileSize returns the maximum file size allowed for tool operations.
func (c ToolsConfig) MaxFileSize() int64 { return c.maxFileSize }

// MaxIterations returns the maximum number of tool iterations per request.
func (c ToolsConfig) MaxIterations() int { return c.maxIterations }

// PermissionDefault returns the default permission level for tools.
func (c ToolsConfig) PermissionDefault() string { return c.permissionDefault }

// ToolPermissions returns a copy of the tool-specific permission levels.
func (c ToolsConfig) ToolPermissions() map[string]string {
	out := make(map[string]string, len(c.toolPermissions))
	maps.Copy(out, c.toolPermissions)
	return out
}

type toolsDTO struct {
	Permissions   permissionsDTO `json:"permissions"`
	MaxFileSize   int64          `json:"max_file_size"`
	MaxIterations int            `json:"max_iterations"`
}

type permissionsDTO struct {
	ByTool  map[string]string `json:"by_tool"`
	Default string            `json:"default"`
}

package domain

// Provider IDs
const (
	ProviderGoogle = "google"
)

// Auth Method and Field IDs
const (
	AuthMethodAPIKey = "api_key"
	AuthMethodEnv    = "env"
	AuthFieldAPIKey   = "api_key"
	AuthFieldProject  = "project"
	AuthFieldLocation = "location"
)

// UI and Formatting
const (
	ModelIDSeparator = "/"
)

// Application Metadata
const (
	AppName       = "iav"
	ConfigBaseDir = ".config"
)

// Standard Permissions (Unix)
const (
	DefaultDirPerm  = 0o755
	DefaultFilePerm = 0o644
	PrivateDirPerm  = 0o700
	PrivateFilePerm = 0o600
)

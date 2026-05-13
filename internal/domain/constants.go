package domain

// Provider IDs.
const (
	ProviderGoogle = "google"
	ProviderGitHub = "github"
)

// Auth Method and Field IDs.
const (
	AuthMethodAPIKey = "api_key"
	AuthMethodEnv    = "env"
	AuthFieldAPIKey  = "api_key"
)

// UI and Formatting.
const (
	ModelIDSeparator = "/"
)

// Message metadata keys.
const (
	NotificationMessageExtraKey = "iav/is_notification"
)

// Application Metadata.
const (
	AppName       = "iav"
	ConfigBaseDir = ".config"
)

// Standard Permissions (Unix).
const (
	DefaultDirPerm  = 0o755
	DefaultFilePerm = 0o644
	PrivateDirPerm  = 0o700
	PrivateFilePerm = 0o600
)

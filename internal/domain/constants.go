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
	// CancelMessageExtraKey marks the synthetic user message appended on session cancel (LLM-facing).
	// History view does not print its content; it shows a gutter marker on the preceding assistant block.
	CancelMessageExtraKey = "iav/is_cancel_message"
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

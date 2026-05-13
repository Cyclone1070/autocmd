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

// Message metadata keys.
const (
	NotificationMessageExtraKey = "iav/is_notification"
	// ThoughtDurationMsExtraKey is Message.Extra: ms from first reasoning chunk to stream end (persisted for history UI).
	ThoughtDurationMsExtraKey = "iav/reasoning_phase_duration_ms"
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
	PrivateFilePerm = 0o600
)

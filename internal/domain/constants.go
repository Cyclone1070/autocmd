package domain

// Provider IDs.
const (
	ProviderGoogle   = "google"
	ProviderGitHub   = "github"
	ProviderOpenCode = "opencode"
)

// ID Generation Constants.
const (
	ShortIDLength       = 8
	MaxCollisionRetries = 100
)

// Auth Method and Field IDs.
const (
	AuthMethodAPIKey = "api_key"
	AuthMethodEnv    = "env"
	AuthFieldAPIKey  = "api_key"
)

// Message metadata keys.
const (
	NotificationMessageExtraKey = "autocmd/is_notification"
	// ThoughtDurationMsExtraKey is Message.Extra: ms from first reasoning chunk to stream end (persisted for history UI).
	ThoughtDurationMsExtraKey = "autocmd/reasoning_phase_duration_ms"
)

// Application Metadata.
const (
	AppName       = "autocmd"
	ConfigBaseDir = ".config"
)

// Standard Permissions (Unix).
const (
	DefaultDirPerm  = 0o755
	DefaultFilePerm = 0o644
	PrivateFilePerm = 0o600
)

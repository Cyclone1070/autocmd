package cmd

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/llm"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/state"
)

// App holds the core dependencies for the CLI commands.
type Deps struct {
	Config       *config.Config
	State        *state.State
	StateManager *state.Manager
	SessionStore *session.Store
	AuthManager  *auth.Manager
	OAuthManager *auth.OAuthManager
	LLMRegistry  *llm.Registry
	BootstrapFS  fs.FileSystem
}

// Wire initializes the standard application dependency stack.
func Wire() (*Deps, error) {
	// Bootstrap FS used for config, state, and session metadata.
	bootstrapFS := fs.NewOSFileSystem(-1)

	configMgr := config.NewManager(bootstrapFS)
	cfg, err := configMgr.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	stateMgr := state.NewManager(bootstrapFS)
	appState, err := stateMgr.Load()
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	authMgr, err := buildAuthManager(cfg)
	if err != nil {
		return nil, err
	}

	sessionStore, err := buildSessionStore(cfg, bootstrapFS)
	if err != nil {
		return nil, err
	}

	return &Deps{
		Config:       cfg,
		State:        appState,
		StateManager: stateMgr,
		SessionStore: sessionStore,
		AuthManager:  authMgr,
		OAuthManager: auth.NewOAuthManager(nil),
		LLMRegistry:  buildLLMRegistry(authMgr),
		BootstrapFS:  bootstrapFS,
	}, nil
}

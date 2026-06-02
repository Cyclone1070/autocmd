package cmd

import (
	"fmt"

	"github.com/Cyclone1070/autocmd/internal/auth"
	"github.com/Cyclone1070/autocmd/internal/command"
	"github.com/Cyclone1070/autocmd/internal/config"
	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/fs"
	"github.com/Cyclone1070/autocmd/internal/provider"
	"github.com/Cyclone1070/autocmd/internal/session"
	"github.com/Cyclone1070/autocmd/internal/state"
	"github.com/Cyclone1070/autocmd/internal/tool/service/path"
	"github.com/Cyclone1070/autocmd/internal/ui"
)

// Deps holds the core dependencies for the CLI commands.
type Deps struct {
	Config           *config.Config
	State            *domain.State
	StateMgr         *state.Manager
	SessionStore     *session.Store
	CommandStore     *command.Store
	AuthManager      *auth.Manager
	OAuthManager     *auth.OAuthManager
	LLMRegistry      *provider.LLMRegistry
	ProviderRegistry *provider.Registry
	BootstrapFS      fs.FileSystem
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

	llmRegistry, providerRegistry := buildRegistries(cfg, authMgr)

	sessionStore, err := buildSessionStore(bootstrapFS)
	if err != nil {
		return nil, err
	}

	commandStore, err := buildCommandStore(bootstrapFS)
	if err != nil {
		return nil, err
	}

	return &Deps{
		Config:           cfg,
		State:            appState,
		StateMgr:         stateMgr,
		SessionStore:     sessionStore,
		CommandStore:     commandStore,
		AuthManager:      authMgr,
		OAuthManager:     auth.NewOAuthManager(nil),
		LLMRegistry:      llmRegistry,
		ProviderRegistry: providerRegistry,
		BootstrapFS:      bootstrapFS,
	}, nil
}

func buildFS(cfg *config.Config) *fs.OSFileSystem {
	return fs.NewOSFileSystem(cfg.Tools().MaxFileSize())
}

func buildPathResolver() (*path.Resolver, error) {
	canonicalRoot, err := path.CanonicaliseRoot(path.OSFileSystem{}, ".")
	if err != nil {
		return nil, err
	}
	return path.NewResolver(canonicalRoot), nil
}

func buildSessionStore(filesystem fs.FileSystem) (*session.Store, error) {
	storageDir, err := session.DefaultStorageDir()
	if err != nil {
		return nil, err
	}
	return session.NewStore(filesystem, storageDir), nil
}

func buildCommandStore(filesystem fs.FileSystem) (*command.Store, error) {
	storagePath, err := command.DefaultStorePath()
	if err != nil {
		return nil, err
	}
	return command.NewStore(filesystem, storagePath), nil
}

// buildRegistries creates the Provider and LLM registries with injected model lists.
func buildRegistries(cfg *config.Config, authMgr *auth.Manager) (*provider.LLMRegistry, *provider.Registry) {
	googleModels := toDomainModels(cfg.Providers()[domain.ProviderGoogle])
	githubModels := toDomainModels(cfg.Providers()[domain.ProviderGitHub])
	opencodeModels := toDomainModels(cfg.Providers()[domain.ProviderOpenCode])

	providerRegistry := provider.NewRegistry(
		authMgr,
		provider.NewGoogleProvider(googleModels),
		provider.NewGitHubProvider(githubModels),
		provider.NewOpenCodeProvider(opencodeModels),
	)

	llmRegistry := provider.NewLLMRegistry(authMgr, providerRegistry)

	return llmRegistry, providerRegistry
}

func toDomainModels(configs []config.ModelConfig) []domain.LLMInfo {
	models := make([]domain.LLMInfo, 0, len(configs))
	for _, m := range configs {
		models = append(models, domain.LLMInfo{
			ID:            m.ID,
			DisplayName:   m.Name,
			ContextWindow: m.ContextWindow,
		})
	}
	return models
}

func buildAuthManager(cfg *config.Config) (*auth.Manager, error) {
	osFS := buildFS(cfg)
	storePath, err := auth.DefaultStorePath()
	if err != nil {
		return nil, err
	}
	return auth.NewManager(osFS, storePath), nil
}

// newTheme constructs a ui.Theme from config.UIConfig.
func newTheme(cfg config.UIConfig) *ui.Theme {
	return &ui.Theme{
		PrimaryCol: ui.ToAdaptiveColor(cfg.PrimaryColor()),
		SuccessCol: ui.ToAdaptiveColor(cfg.SuccessColor()),
		ErrorCol:   ui.ToAdaptiveColor(cfg.ErrorColor()),
		MutedCol:   ui.ToAdaptiveColor(cfg.MutedColor()),
		TextCol:    ui.ToAdaptiveColor(cfg.TextColor()),
	}
}

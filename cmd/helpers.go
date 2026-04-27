package cmd

import (
	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

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

func buildSessionStore(cfg *config.Config, filesystem fs.FileSystem) *session.Store {
	return session.NewStore(filesystem, cfg.Session().StorageDir())
}

// buildRegistries creates the Provider and LLM registries with injected model lists.
func buildRegistries(cfg *config.Config, authMgr *auth.Manager) (*provider.LLMRegistry, *provider.ProviderRegistry) {
	googleModels := toDomainModels(cfg.Providers()["google"])
	githubModels := toDomainModels(cfg.Providers()["github"])

	providerRegistry := provider.NewProviderRegistry(
		authMgr,
		provider.NewGoogleProvider(googleModels),
		provider.NewGitHubProvider(githubModels),
	)

	llmRegistry := provider.NewLLMRegistry(authMgr, providerRegistry)

	return llmRegistry, providerRegistry
}

func toDomainModels(configs []config.ModelConfig) []domain.LLMInfo {
	var models []domain.LLMInfo
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

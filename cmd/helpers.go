package cmd

import (
	"os"

	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/fs"
	"github.com/Cyclone1070/iav/internal/llm"
	"github.com/Cyclone1070/iav/internal/llm/google"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

func buildFS(cfg *config.Config) *fs.OSFileSystem {
	return fs.NewOSFileSystem(cfg)
}

func buildPathResolver() (*path.Resolver, error) {
	canonicalRoot, err := path.CanonicaliseRoot(path.OSFileSystem{}, ".")
	if err != nil {
		return nil, err
	}
	return path.NewResolver(canonicalRoot), nil
}

func buildSessionStore(cfg *config.Config) (*session.Store, error) {
	fileSystem := buildFS(cfg)
	return session.NewStore(cfg, fileSystem), nil
}

func buildLLMRegistry() *llm.Registry {
	return llm.NewRegistry(google.NewProvider())
}

func buildAuthManager(cfg *config.Config) (*auth.Manager, error) {
	osFS := fs.NewOSFileSystem(cfg)
	storePath, err := auth.DefaultStorePath()
	if err != nil {
		return nil, err
	}
	return auth.NewManager(osFS, storePath), nil
}

func resolveCredential(authMgr *auth.Manager, providerID string) *domain.Credential {
	// Priority 1: auth.json (from iav auth)
	cred, err := authMgr.Get(providerID)
	if err == nil && cred != nil {
		// Only return if it actually has an API key (avoid "zombie" entries)
		if cred.APIKey != "" {
			return cred
		}
	}

	// Priority 2: Environment Variables (Fallback)
	if providerID == domain.ProviderGoogle {
		if key := os.Getenv("GEMINI_API_KEY"); key != "" {
			return &domain.Credential{Type: domain.AuthMethodEnv, APIKey: key}
		}
	}

	return nil
}

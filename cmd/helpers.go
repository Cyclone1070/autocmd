package cmd

import (
	"github.com/Cyclone1070/iav/internal/auth"
	"github.com/Cyclone1070/iav/internal/config"
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

// buildLLMRegistry creates the LLM registry with supported providers.
func buildLLMRegistry(authMgr *auth.Manager) *llm.Registry {
	return llm.NewRegistry(authMgr, google.NewProvider())
}

func buildAuthManager(cfg *config.Config) (*auth.Manager, error) {
	osFS := fs.NewOSFileSystem(cfg)
	storePath, err := auth.DefaultStorePath()
	if err != nil {
		return nil, err
	}
	return auth.NewManager(osFS, storePath), nil
}

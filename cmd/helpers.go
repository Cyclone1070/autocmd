package cmd

import (
	"context"
	"fmt"
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
	canonicalRoot, err := path.CanonicaliseRoot(".")
	if err != nil {
		return nil, err
	}
	return path.NewResolver(canonicalRoot), nil
}

func buildSessionStore(cfg *config.Config) (*session.Store, error) {
	fileSystem := buildFS(cfg)
	return session.NewStore(cfg, fileSystem), nil
}

func buildLLMRegistry(ctx context.Context, cfg *config.Config) (*llm.Registry, error) {
	cred := resolveCredential("google")
	if cred == nil {
		return nil, fmt.Errorf("no credentials found for google. Set GEMINI_API_KEY")
	}

	googleProvider, err := google.NewProvider(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create google provider: %w", err)
	}

	return llm.NewRegistry(googleProvider), nil
}

func resolveCredential(providerID string) *domain.Credential {
	if providerID == "google" {
		if key := os.Getenv("GEMINI_API_KEY"); key != "" {
			return &domain.Credential{Type: "api_key", APIKey: key}
		}
	}

	cred, _ := auth.Get(providerID)
	return cred
}

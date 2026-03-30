package domain

// LLM and provider interfaces and listing metadata.

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

// LLM is a language model instance powered by Eino.
type LLM interface {
	ID() string
	DisplayName() string
	ContextWindow() int
	Model() model.ToolCallingChatModel
}

// LLMInfo is metadata for listing language models.
type LLMInfo struct {
	ID            string
	DisplayName   string
	ContextWindow int
}

// Provider represents an LLM service (e.g., Google, OpenAI).
// It acts as a factory and metadata source for authentication and model listing.
type Provider interface {
	ID() string
	SupportedAuthMethods() []AuthMethod
	List() []LLMInfo
	GetLLM(ctx context.Context, cred *Credential, info LLMInfo) (LLM, error)
}

// ProviderInfo contains a provider's ID and its resolved credential (if any).
type ProviderInfo struct {
	ID         string
	Credential *Credential
}

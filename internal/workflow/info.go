package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

type infoLLMRegistry interface {
	ListProviders(ctx context.Context) ([]domain.ProviderInfo, error)
	Get(ctx context.Context, id string) (domain.LLM, error)
}

type infoState interface {
	Model() string
	CurrentSessionID() string
}

type infoSessionStore interface {
	Get(id string) (*domain.Session, error)
}

// InfoWorkflow gathers information about the current configuration and state.
type InfoWorkflow struct {
	registry infoLLMRegistry
	state    infoState
	store    infoSessionStore
}

// NewInfoWorkflow creates a new InfoWorkflow.
func NewInfoWorkflow(registry infoLLMRegistry, state infoState, store infoSessionStore) *InfoWorkflow {
	return &InfoWorkflow{
		registry: registry,
		state:    state,
		store:    store,
	}
}

// Gather executes the info workflow results.
func (w *InfoWorkflow) Gather(ctx context.Context) (*domain.SystemSnapshot, error) {
	res := &domain.SystemSnapshot{}

	// 1. Authorized Providers
	providers, err := w.registry.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		if p.Credential != nil {
			res.Authorized = append(res.Authorized, fmt.Sprintf("%s (%s)", p.ID, p.Credential.Type))
		}
	}

	// 2. Model from State
	res.Model = w.state.Model()

	// 3. Session Info
	sessionID := w.state.CurrentSessionID()
	var sessionMessages domain.Messages
	if sessionID != "" {
		sess, err := w.store.Get(sessionID)
		if err == nil {
			res.SessionDisplay = sess.Name
			if res.SessionDisplay == "" {
				res.SessionDisplay = sess.ID
			}
			sessionMessages = sess.Messages
		} else {
			res.SessionDisplay = fmt.Sprintf("%s (not found)", sessionID)
		}
	} else {
		res.SessionDisplay = "none"
	}

	// 4. LLM Specific Info (Context Window & Tokens)
	if res.Model != "" {
		llm, err := w.registry.Get(ctx, res.Model)
		if err == nil {
			res.ContextWindow = llm.ContextWindow()
			if len(sessionMessages) > 0 {
				tokens, err := llm.ComputeTokens(ctx, sessionMessages)
				if err == nil {
					res.SessionTokens = tokens
				}
			}
		}
	}

	return res, nil
}

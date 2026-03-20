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

type infoBus interface {
	SendUIUpdate(domain.UIUpdate)
}

// InfoWorkflow gathers information about the current configuration and state.
type InfoWorkflow struct {
	registry infoLLMRegistry
	state    infoState
	store    infoSessionStore
}

type InfoDeps struct {
	Bus      infoBus
	Registry infoLLMRegistry
	State    infoState
	Store    infoSessionStore
}

// RunInfo executes the info gathering process asynchronously.
func RunInfo(ctx context.Context, deps *InfoDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := NewInfoWorkflow(deps.Registry, deps.State, deps.Store)
		res, err := wf.gather(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(res)
		deps.Bus.SendUIUpdate(domain.DoneEvent{})
		done <- nil
	}()
	return done
}

// NewInfoWorkflow creates a new InfoWorkflow.
func NewInfoWorkflow(registry infoLLMRegistry, state infoState, store infoSessionStore) *InfoWorkflow {
	return &InfoWorkflow{
		registry: registry,
		state:    state,
		store:    store,
	}
}

// gather executes the info workflow results.
func (w *InfoWorkflow) gather(ctx context.Context) (domain.InfoEvent, error) {
	res := domain.InfoEvent{}

	// 1. Authorized Providers
	providers, err := w.registry.ListProviders(ctx)
	if err != nil {
		return domain.InfoEvent{}, fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		if p.Credential != nil {
			res.Authorized = append(res.Authorized, fmt.Sprintf("%s (%s)", p.ID, p.Credential.Type))
		}
	}

	// 2. Model ID from State
	modelID := w.state.Model()

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
	if modelID != "" {
		llm, err := w.registry.Get(ctx, modelID)
		if err == nil {
			res.Model = llm.DisplayName()
			res.ContextWindow = llm.ContextWindow()
			if len(sessionMessages) > 0 {
				tokens, err := llm.ComputeTokens(ctx, sessionMessages)
				if err == nil {
					res.SessionTokens = tokens
				}
			}
		} else {
			// Fallback if not found in registry
			res.Model = modelID
		}
	}

	return res, nil
}

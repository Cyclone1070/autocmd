package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

type infoProviderRegistry interface {
	List(ctx context.Context) ([]domain.ProviderInfo, error)
}

type infoLLMRegistry interface {
	Get(ctx context.Context, id string) (domain.LLM, error)
}

type infoState interface {
	Model() string
}

type infoSessionStore interface {
	GetSession(id string) (*domain.Session, error)
}

type infoBus interface {
	SendUIUpdate(domain.UIUpdate)
}

// InfoWorkflow gathers information about the current configuration and state.
type InfoWorkflow struct {
	providerRegistry infoProviderRegistry
	llmRegistry      infoLLMRegistry
	state            infoState
	store            infoSessionStore
	sessionID        string
}

// InfoDeps contains the dependencies for the system information workflow.
type InfoDeps struct {
	Bus              infoBus
	ProviderRegistry infoProviderRegistry
	LLMRegistry      infoLLMRegistry
	State            infoState
	Store            infoSessionStore
	SessionID        string
}

// RunInfo executes the info gathering process asynchronously.
func RunInfo(ctx context.Context, deps *InfoDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := NewInfoWorkflow(deps.ProviderRegistry, deps.LLMRegistry, deps.State, deps.Store, deps.SessionID)
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
func NewInfoWorkflow(pRegistry infoProviderRegistry, lRegistry infoLLMRegistry, state infoState, store infoSessionStore, sessionID string) *InfoWorkflow {
	return &InfoWorkflow{
		providerRegistry: pRegistry,
		llmRegistry:      lRegistry,
		state:            state,
		store:            store,
		sessionID:        sessionID,
	}
}

// gather executes the info workflow results.
func (w *InfoWorkflow) gather(ctx context.Context) (domain.InfoEvent, error) {
	res := domain.InfoEvent{}

	// 1. Authorized Providers
	providers, err := w.providerRegistry.List(ctx)
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
	sessionID := w.sessionID
	var sess *domain.Session
	if sessionID != "" {
		var err error
		sess, err = w.store.GetSession(sessionID)
		if err == nil {
			res.SessionDisplay = sess.Name
			if res.SessionDisplay == "" {
				res.SessionDisplay = "Untitled"
			}
		} else {
			res.SessionDisplay = fmt.Sprintf("%s (not found)", sessionID)
		}
	} else {
		res.SessionDisplay = "none"
	}

	// 4. LLM Specific Info (Context Window & Tokens)
	if modelID != "" {
		llm, err := w.llmRegistry.Get(ctx, modelID)
		if err == nil {
			res.Model = llm.DisplayName()
			res.ContextWindow = llm.ContextWindow()
			if sess != nil {
				res.SessionTokens = sess.TotalTokens()
			}
		} else {
			// Fallback if not found in registry
			res.Model = modelID
		}
	}

	return res, nil
}

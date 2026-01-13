package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// llmProvider communicates with an LLM.
type llmProvider interface {
	Generate(ctx context.Context, model string, msgs []domain.Message, tools []domain.Declaration) (*domain.Message, error)
	ListModels(ctx context.Context) ([]string, error)
}

// sessionStore manages session persistence.
type sessionStore interface {
	Create() (*domain.Session, error)
	Get(id string) (*domain.Session, error)
	Save(s *domain.Session) error
	List() ([]domain.SessionSummary, error)
	Delete(id string) error
}

// toolRegistry provides tool storage and lookup.
// Implemented by tool.Registry, injected via NewWorkflow.
type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

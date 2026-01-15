package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// modelRegistry resolves model IDs to Model instances.
type modelRegistry interface {
	Get(ctx context.Context, id string) (domain.Model, error)
	List(ctx context.Context) ([]domain.ModelInfo, error)
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

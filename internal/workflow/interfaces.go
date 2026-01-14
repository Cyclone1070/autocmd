package workflow

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// providerRegistry manages multiple LLM providers.
type providerRegistry interface {
	Get(name string) (domain.Provider, bool)
	List() []string
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

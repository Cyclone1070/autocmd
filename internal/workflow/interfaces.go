package workflow

import (
	"context"
	"encoding/json"

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

// Tool defines the interface for individual tools.
type Tool interface {
	Name() string
	Declaration() domain.Declaration
	Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error)
}

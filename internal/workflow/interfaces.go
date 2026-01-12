package workflow

import (
	"context"
	"encoding/json"

	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/tool"
)

// llmProvider communicates with an LLM.
type llmProvider interface {
	Generate(ctx context.Context, model string, msgs []provider.Message, tools []tool.Declaration) (*provider.Message, error)
	ListModels(ctx context.Context) ([]string, error)
}

// sessionStore manages session persistence.
type sessionStore interface {
	Create() (*session.Session, error)
	Get(id string) (*session.Session, error)
	Save(s *session.Session) error
	List() ([]session.SessionSummary, error)
	Delete(id string) error
}

// Tool defines the interface for individual tools.
type Tool interface {
	Name() string
	Declaration() tool.Declaration
	Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error)
}

package agent

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// toolRegistry provides tool storage and lookup.
type toolRegistry interface {
	Declarations() []domain.Declaration
	Get(name string) (domain.Tool, bool)
}

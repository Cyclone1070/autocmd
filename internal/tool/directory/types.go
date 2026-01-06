package directory

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
)

// -- Directory Tool Contract Types --

// -- Directory Tool Contract Types --

// (ListDirectory types moved to internal/tool/directory/list.go)

// FindFileRequest represents the parameters for a FindFile operation
type FindFileRequest struct {
	Pattern        string `json:"pattern"`
	SearchPath     string `json:"search_path"`
	MaxDepth       int    `json:"max_depth,omitempty"`
	IncludeIgnored bool   `json:"include_ignored,omitempty"`
	Offset         int    `json:"offset,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

func (r *FindFileRequest) Validate(cfg *config.Config) error {
	if r.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
	if r.Limit <= 0 {
		r.Limit = cfg.Tools.DefaultFindFileLimit
	}
	if r.Limit > cfg.Tools.MaxFindFileLimit {
		r.Limit = cfg.Tools.MaxFindFileLimit
	}
	return nil
}

// FindFileResponse contains the result of a FindFile operation
type FindFileResponse struct {
	FormattedMatches string // newline-separated paths
	Offset           int
	Limit            int
	TotalCount       int
	HitMaxResults    bool
}

func (r FindFileResponse) Success() bool {
	return true
}

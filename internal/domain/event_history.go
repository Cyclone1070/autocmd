package domain

import "github.com/cloudwego/eino/schema"

// HistoryEvent contains the full conversation history for a session.
type HistoryEvent struct {
	ToolDisplays map[string]ToolDisplay
	Messages     []*schema.Message
}

func (HistoryEvent) isUIUpdate() {}

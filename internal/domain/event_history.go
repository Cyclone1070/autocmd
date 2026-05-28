package domain

import "github.com/cloudwego/eino/schema"

// HistoryEvent contains the full conversation history for a session.
type HistoryEvent struct {
	ToolDisplays ToolDisplays
	Messages     []*schema.Message
}

func (HistoryEvent) isUIUpdate() {}

package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/cloudwego/eino/schema"
)

// SessionMetadata contains the metadata fields stored in metadata.json.
type SessionMetadata struct {
	ID           string
	Name         string
	MessageCount int
	TokenCount   int
	Created      time.Time
	Updated      time.Time
	WorkingDir   string
	Active       bool
}

// SessionMessages contains the messages stored in messages.json.
type SessionMessages struct {
	Messages []*schema.Message
}

// SessionDisplays contains the tool displays stored in displays.json.
type SessionDisplays struct {
	ToolDisplays map[string]ToolDisplay
}

// Session is the composite session type that bundles all three parts.
// Use Store.GetSession() to load a full session.
type Session struct {
	SessionMetadata
	SessionMessages
	SessionDisplays
}

// TotalTokens returns the factual total tokens in the session as of the last model response.
func (s *Session) TotalTokens() int {
	for i := range slices.Backward(s.Messages) {
		m := s.Messages[i]
		if m.Role == schema.Assistant && m.ResponseMeta != nil && m.ResponseMeta.Usage != nil {
			return m.ResponseMeta.Usage.TotalTokens
		}
	}
	return 0
}

// UnmarshalJSON implements custom JSON unmarshaling for the polymorphic ToolDisplay map.
func (d *SessionDisplays) UnmarshalJSON(data []byte) error {
	var raws map[string]json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	d.ToolDisplays = make(map[string]ToolDisplay)
	for id, raw := range raws {
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			return err
		}

		var display ToolDisplay
		switch peek.Type {
		case toolDisplayTypeString:
			var d StringDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case toolDisplayTypeDiff:
			var d DiffDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case toolDisplayTypeBash:
			var d BashDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		case toolDisplayTypeQuestion:
			var d QuestionDisplay
			if err := json.Unmarshal(raw, &d); err != nil {
				return err
			}
			display = d
		default:
			return fmt.Errorf("unknown display type: %s", peek.Type)
		}
		d.ToolDisplays[id] = display
	}
	return nil
}

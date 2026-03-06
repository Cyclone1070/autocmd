package google

import (
	"encoding/json"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"google.golang.org/genai"
)

func toTools(decls []domain.Declaration) []*genai.Tool {
	if len(decls) == 0 {
		return nil
	}
	var fds []*genai.FunctionDeclaration
	for _, d := range decls {
		fds = append(fds, &genai.FunctionDeclaration{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  toSchema(d.Parameters),
		})
	}
	return []*genai.Tool{{FunctionDeclarations: fds}}
}

func toSchema(s *domain.Schema) *genai.Schema {
	if s == nil {
		return nil
	}
	gs := &genai.Schema{
		Type:        toType(s.Type),
		Description: s.Description,
		Enum:        s.Enum,
		Required:    s.Required,
		Properties:  make(map[string]*genai.Schema),
	}
	for k, v := range s.Properties {
		gs.Properties[k] = toSchema(v)
	}
	if s.Items != nil {
		gs.Items = toSchema(s.Items)
	}
	return gs
}

func toType(t domain.Type) genai.Type {
	switch t {
	case domain.TypeObject:
		return genai.TypeObject
	case domain.TypeArray:
		return genai.TypeArray
	case domain.TypeString:
		return genai.TypeString
	case domain.TypeNumber:
		return genai.TypeNumber
	case domain.TypeInteger:
		return genai.TypeInteger
	case domain.TypeBoolean:
		return genai.TypeBoolean
	default:
		return genai.TypeString
	}
}

// historyResult contains the converted message history for Gemini.
type historyResult struct {
	SystemPrompt string
	Contents     []*genai.Content
}

// toHistory converts domain messages to genai Content.
// It returns a combined system prompt and the full list of contents.
func toHistory(msgs []domain.Message) (historyResult, error) {
	if len(msgs) == 0 {
		return historyResult{}, fmt.Errorf("empty message list")
	}

	var result historyResult

	for _, m := range msgs {
		if sm, ok := m.(domain.SystemMessage); ok {
			if result.SystemPrompt != "" {
				result.SystemPrompt += "\n"
			}
			result.SystemPrompt += sm.Content
			continue
		}

		parts, err := toParts(m)
		if err != nil {
			return historyResult{}, err
		}

		role := "user"
		switch m.(type) {
		case domain.AssistantMessage:
			role = "model"
		case domain.ToolMessage:
			role = "function"
		}

		result.Contents = append(result.Contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	return result, nil
}

func toParts(m domain.Message) ([]*genai.Part, error) {
	var parts []*genai.Part

	switch msg := m.(type) {
	case domain.UserMessage:
		if msg.Content != "" {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}
	case domain.AssistantMessage:
		if msg.Content != "" {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			var args map[string]any
			if len(tc.Arguments) > 0 {
				if err := json.Unmarshal(tc.Arguments, &args); err != nil {
					return nil, fmt.Errorf("invalid tool arguments json: %w", err)
				}
			}
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: tc.Name,
					Args: args,
				},
			})
		}
	case domain.ToolMessage:
		resp := map[string]any{
			"result": msg.Content,
		}
		parts = append(parts, &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     msg.ToolName,
				Response: resp,
			},
		})
	case domain.SystemMessage:
		// System messages are handled in toHistory, but for completeness:
		if msg.Content != "" {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}
	}

	return parts, nil
}

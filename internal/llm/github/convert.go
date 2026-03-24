package github

import (
	"encoding/json"

	"github.com/Cyclone1070/iav/internal/domain"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// toSDKMessages converts domain messages to OpenAI SDK message params,
// automatically applying normalization for strict role alternation.
func toSDKMessages(msgs domain.Messages) []openai.ChatCompletionMessageParamUnion {
	normalized := normalizeHistory(msgs)
	var results []openai.ChatCompletionMessageParamUnion

	for _, m := range normalized {
		switch v := m.(type) {
		case domain.SystemMessage:
			results = append(results, openai.SystemMessage(v.Content))
		case domain.UserMessage:
			results = append(results, openai.UserMessage(v.Content))
		case domain.AssistantMessage:
			asst := openai.ChatCompletionAssistantMessageParam{}
			if v.Content != "" {
				asst.Content.OfString = param.NewOpt(v.Content)
			}
			if len(v.ToolCalls) > 0 {
				for _, tc := range v.ToolCalls {
					asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					})
				}
			}
			results = append(results, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
		case domain.ToolMessage:
			results = append(results, openai.ToolMessage(v.Content, v.ToolCallID))
		}
	}
	return results
}

// toSDKTools converts domain tool declarations to OpenAI SDK tool params.
func toSDKTools(decls []domain.Declaration) []openai.ChatCompletionToolUnionParam {
	var tools []openai.ChatCompletionToolUnionParam
	for _, d := range decls {
		var smap map[string]any
		if d.Parameters != nil {
			data, _ := json.Marshal(d.Parameters)
			_ = json.Unmarshal(data, &smap)
		}
		tools = append(tools, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        d.Name,
			Description: openai.String(d.Description),
			Parameters:  shared.FunctionParameters(smap),
		}))
	}
	return tools
}

// normalizeHistory ensures strict role alternation by merging consecutive same-role messages.
func normalizeHistory(msgs domain.Messages) domain.Messages {
	if len(msgs) == 0 {
		return msgs
	}

	var normalized domain.Messages
	for _, m := range msgs {
		if len(normalized) == 0 {
			normalized = append(normalized, m)
			continue
		}

		last := normalized[len(normalized)-1]

		if last.Role() == m.Role() {
			switch lastMsg := last.(type) {
			case domain.UserMessage:
				if newMsg, ok := m.(domain.UserMessage); ok {
					normalized[len(normalized)-1] = domain.UserMessage{
						Content: lastMsg.Content + "\n\n" + newMsg.Content,
					}
					continue
				}
			case domain.AssistantMessage:
				if newMsg, ok := m.(domain.AssistantMessage); ok {
					merged := lastMsg
					merged.Content += "\n\n" + newMsg.Content
					merged.ToolCalls = append(merged.ToolCalls, newMsg.ToolCalls...)
					normalized[len(normalized)-1] = merged
					continue
				}
			case domain.SystemMessage:
				if newMsg, ok := m.(domain.SystemMessage); ok {
					normalized[len(normalized)-1] = domain.SystemMessage{
						Content: lastMsg.Content + "\n\n" + newMsg.Content,
					}
					continue
				}
			}
		}

		normalized = append(normalized, m)
	}

	return normalized
}

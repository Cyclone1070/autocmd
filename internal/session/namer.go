package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// GenerateName creates a short title for a session based on the provided text.
// It uses the provided LLM to generate the title.
func GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {

	prompt := fmt.Sprintf("Summarize this in 3-5 words as a conversation title. Your response must only be the title and nothing else: %s", target)

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	resp, err := llm.Model().Generate(ctx, messages)
	if err != nil || resp == nil {
		return fallbackName(target), nil
	}

	name := strings.TrimSpace(resp.Content)
	if name == "" {
		return fallbackName(target), nil
	}

	// Remove any surrounding quotes the LLM might have included
	name = strings.Trim(name, "\"")

	return name, nil
}

func fallbackName(msg string) string {
	const maxLen = 50
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

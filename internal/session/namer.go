package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// GenerateName creates a short title for a session based on the provided text.
// It uses the provided LLM to generate the title.
func GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {

	prompt := fmt.Sprintf("Summarize this in 3-5 words as a conversation title. Your response must only be the title and nothing else: %s", target)

	messages := domain.Messages{
		domain.UserMessage{Content: prompt},
	}

	stream, err := llm.Stream(ctx, messages, nil)
	if err != nil || stream == nil {
		return fallbackName(target), nil
	}

	var sb strings.Builder
	for stream.Next() {
		if chunk, ok := stream.Chunk().(domain.TextChunk); ok {
			sb.WriteString(chunk.Text)
		}
	}

	if err := stream.Err(); err != nil {
		return fallbackName(target), nil
	}

	name := strings.TrimSpace(sb.String())
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

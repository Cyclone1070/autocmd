package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Cyclone1070/iav/internal/domain"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

type copilotLLM struct {
	tokenSource   *TokenSource
	modelID       string
	displayName   string
	contextWindow int
}

func newCopilotLLM(tokenSource *TokenSource, modelID string, displayName string, contextWindow int) *copilotLLM {
	return &copilotLLM{
		tokenSource:   tokenSource,
		modelID:       modelID,
		displayName:   displayName,
		contextWindow: contextWindow,
	}
}

func (l *copilotLLM) ID() string {
	return domain.ProviderGitHub + domain.ModelIDSeparator + l.modelID
}

func (l *copilotLLM) DisplayName() string {
	return l.displayName
}

func (l *copilotLLM) ContextWindow() int {
	return l.contextWindow
}

func (l *copilotLLM) ComputeTokens(ctx context.Context, msgs domain.Messages) (int, error) {
	// Simple approximation (characters / 4)
	count := 0
	for _, m := range msgs {
		switch v := m.(type) {
		case domain.UserMessage:
			count += len(v.Content) / 4
		case domain.AssistantMessage:
			count += len(v.Content) / 4
		case domain.SystemMessage:
			count += len(v.Content) / 4
		}
	}
	return count, nil
}

func (l *copilotLLM) getClient(ctx context.Context) (*openai.Client, error) {
	token, err := l.tokenSource.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	client := openai.NewClient(
		option.WithBaseURL("https://api.githubcopilot.com/"),
		option.WithHeader("X-Github-Api-Version", "2023-07-07"),
		option.WithHeader("User-Agent", "GithubCopilot/1.155.0"),
		option.WithHeader("Editor-Version", "vscode/1.85.1"),
		option.WithHeader("Editor-Plugin-Version", "copilot/1.155.0"),
		option.WithAPIKey(token),
	)
	return &client, nil
}

func (l *copilotLLM) Stream(ctx context.Context, msgs domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	client, err := l.getClient(ctx)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(l.modelID),
		Messages: toSDKMessages(msgs),
	}

	if len(tools) > 0 {
		params.Tools = toSDKTools(tools)
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	return &copilotStream{stream: stream}, nil
}

type copilotStream struct {
	stream *ssestream.Stream[openai.ChatCompletionChunk]
	err    error
	curr   domain.StreamChunk
}

func (s *copilotStream) Next() bool {
	if s.err != nil {
		return false
	}

	if !s.stream.Next() {
		if err := s.stream.Err(); err != nil && !errors.Is(err, io.EOF) {
			s.err = err
		}
		return false
	}

	chunk := s.stream.Current()
	if len(chunk.Choices) == 0 {
		return s.Next()
	}

	delta := chunk.Choices[0].Delta
	if delta.Content != "" {
		s.curr = domain.TextChunk{Text: delta.Content}
		return true
	}

	if len(delta.ToolCalls) > 0 {
		tc := delta.ToolCalls[0]
		// Map function calls to domain tool calls
		if tc.Function.Name != "" || tc.Function.Arguments != "" {
			s.curr = domain.ToolCall{
				Index:     int(tc.Index),
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			}
			return true
		}
	}

	return s.Next()
}

func (s *copilotStream) Chunk() domain.StreamChunk {
	return s.curr
}

func (s *copilotStream) Err() error {
	return s.err
}

func (s *copilotStream) Close() error {
	return s.stream.Close()
}

package agent

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Loop is the central orchestrator for the execution loop.
type Loop struct {
	llm          domain.LLM
	toolExecutor *toolExecutor
	events       chan<- domain.Event
	cfg          *config.Config
}

// NewLoop creates a new Loop with its dependencies.
func NewLoop(
	llm domain.LLM,
	toolRegistry toolRegistry,
	cfg *config.Config,
	events chan<- domain.Event,
) *Loop {
	if cfg == nil {
		panic("cfg is required")
	}
	return &Loop{
		llm:          llm,
		toolExecutor: newToolExecutor(toolRegistry),
		events:       events,
		cfg:          cfg,
	}
}

// Run executes the main LLM-tool interaction loop.
func (l *Loop) Run(ctx context.Context, session *domain.Session, input string) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}

	session.Messages = append(session.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: input,
	})

	defer func() {
		if ctx.Err() != nil {
			msgs := session.Messages
			if len(msgs) == 0 || msgs[len(msgs)-1].Content != "[Session cancelled by user]" {
				session.Messages = append(session.Messages, domain.Message{
					Role:    domain.RoleUser,
					Content: "[Session cancelled by user]",
				})
			}
		}

		if l.events != nil {
			select {
			case l.events <- domain.DoneEvent{}:
			default:
			}
		}
	}()

	maxIterations := l.cfg.Tools.MaxIterations
	for range maxIterations {
		if err := ctx.Err(); err != nil {
			return err
		}

		if l.events != nil {
			l.events <- domain.ThinkingEvent{}
		}

		stream, err := l.llm.Stream(ctx, session.Messages, l.toolExecutor.declarations())
		if err != nil {
			return fmt.Errorf("LLM.Stream: %w", err)
		}

		var msg domain.Message
		msg.Role = domain.RoleAssistant

		for stream.Next() {
			switch c := stream.Chunk().(type) {
			case domain.TextChunk:
				msg.Content += c.Text
				if l.events != nil {
					l.events <- domain.TextEvent{Text: c.Text}
				}
			case domain.ToolCall:
				msg.ToolCalls = append(msg.ToolCalls, c)
			}
		}

		// Save the accumulated stream contents so far before checking for errors
		session.Messages = append(session.Messages, msg)

		if err := stream.Err(); err != nil {
			return fmt.Errorf("stream.Err: %w", err)
		}

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, tc := range msg.ToolCalls {
			toolResp, err := l.toolExecutor.execute(ctx, tc, l.events)
			if err != nil {
				return fmt.Errorf("tools.Execute (%s): %w", tc.Name, err)
			}
			session.Messages = append(session.Messages, toolResp)
		}
	}

	session.Messages = append(session.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: "[Max iterations reached]",
	})

	return fmt.Errorf("max iterations (%d) reached", maxIterations)
}

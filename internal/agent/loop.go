package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

// Loop is the central orchestrator for the execution loop.
type Loop struct {
	llm          domain.LLM
	toolExecutor *toolExecutor
	events       eventSender
	cfg          *config.Config
}

// NewLoop creates a new Loop with its dependencies.
func NewLoop(
	llm domain.LLM,
	toolRegistry toolRegistry,
	cfg *config.Config,
	events eventSender,
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
			// DoneEvent is now emitted by the caller (root.go)
		}
	}()

	maxIterations := l.cfg.Tools.MaxIterations
	for range maxIterations {
		if err := ctx.Err(); err != nil {
			return err
		}

		if l.events != nil {
			l.events.Send(domain.ThinkingEvent{})
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
					l.events.Send(domain.TextEvent(c))
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

		var wg sync.WaitGroup
		var mu sync.Mutex
		toolResponses := make([]domain.Message, len(msg.ToolCalls))

		for i, tc := range msg.ToolCalls {
			wg.Add(1)
			go func(idx int, call domain.ToolCall) {
				defer wg.Done()

				resp, disp, err := l.toolExecutor.execute(ctx, call, l.events)
				if err != nil {
					// We've already handled individual tool errors inside toolExecutor.execute
					// which returns a domain.Message with the error for the LLM.
					// If there's a serious infrastructure error (context cancelled), we stop.
					return
				}

				mu.Lock()
				defer mu.Unlock()

				if disp != nil {
					if msg.ToolDisplays == nil {
						msg.ToolDisplays = make(map[string]domain.ToolDisplay)
					}
					msg.ToolDisplays[call.ID] = disp
				}
				toolResponses[idx] = resp
			}(i, tc)
		}

		wg.Wait()

		// Final update to the assistant message that launched the calls
		session.Messages[len(session.Messages)-1] = msg
		// Append all responses in the correct order
		for _, r := range toolResponses {
			if r.Role != "" {
				session.Messages = append(session.Messages, r)
			}
		}
	}

	session.Messages = append(session.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: "[Max iterations reached]",
	})

	return fmt.Errorf("max iterations (%d) reached", maxIterations)
}

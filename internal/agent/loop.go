package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// Loop is the central orchestrator for the execution prompt.
type Loop struct {
	llm           domain.LLM
	toolExecutor  *toolExecutor
	events        eventSender
	maxIterations int
}

// NewLoop creates a new Loop with its dependencies.
func NewLoop(
	llm domain.LLM,
	toolRegistry toolRegistry,
	maxIterations int,
	events eventSender,
) *Loop {
	return &Loop{
		llm:           llm,
		toolExecutor:  newToolExecutor(toolRegistry),
		events:        events,
		maxIterations: maxIterations,
	}
}

// Run executes the main LLM-tool interaction prompt.
func (l *Loop) Run(ctx context.Context, session *domain.Session, input string) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}

	session.Messages = append(session.Messages, &schema.Message{
		Role:    schema.User,
		Content: input,
	})

	defer func() {
		if ctx.Err() != nil {
			msgs := session.Messages
			if len(msgs) == 0 {
				return
			}
			lastMsg := msgs[len(msgs)-1]
			if lastMsg.Role != schema.User || lastMsg.Content != "[Session cancelled by user]" {
				session.Messages = append(session.Messages, &schema.Message{
					Role:    schema.User,
					Content: "[Session cancelled by user]",
				})
			}
		}
	}()

	for range l.maxIterations {
		if err := ctx.Err(); err != nil {
			return err
		}

		if l.events != nil {
			l.events.SendUIUpdate(domain.ThinkingEvent{})
		}

		// Bind tools to the model
		modelWithTools, err := l.llm.Model().WithTools(l.toolExecutor.definitions())
		if err != nil {
			return fmt.Errorf("bind tools: %w", err)
		}

		reader, err := modelWithTools.Stream(ctx, session.Messages)
		if err != nil {
			return fmt.Errorf("LLM.Stream: %w", err)
		}
		defer reader.Close()

		var chunks []*schema.Message
		for {
			chunk, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("reader.Recv: %w", err)
			}

			chunks = append(chunks, chunk)

			// Stream fragments to UI
			if l.events != nil {
				if chunk.Content != "" {
					l.events.SendUIUpdate(domain.TextEvent{
						Text:      chunk.Content,
						IsThought: false,
					})
				}
				if chunk.ReasoningContent != "" {
					l.events.SendUIUpdate(domain.TextEvent{
						Text:      chunk.ReasoningContent,
						IsThought: true,
					})
				}
			}
		}

		msg, err := schema.ConcatMessages(chunks)
		if err != nil {
			return fmt.Errorf("ConcatMessages: %w", err)
		}

		// Save the accumulated stream contents so far
		session.Messages = append(session.Messages, msg)

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		toolResponses := make([]*schema.Message, len(msg.ToolCalls))

		for i, tc := range msg.ToolCalls {
			wg.Add(1)
			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				resp, disp, err := l.toolExecutor.execute(ctx, &call, l.events)

				mu.Lock()
				defer mu.Unlock()

				if disp != nil {
					if session.ToolDisplays == nil {
						session.ToolDisplays = make(domain.ToolDisplays)
					}
					session.ToolDisplays[call.ID] = disp
				}
				if resp != nil {
					toolResponses[idx] = resp
				}

				if err != nil {
					return
				}
			}(i, tc)
		}

		wg.Wait()

		// Append all responses in the correct order
		for _, r := range toolResponses {
			if r != nil {
				session.Messages = append(session.Messages, r)
			}
		}
	}

	session.Messages = append(session.Messages, &schema.Message{
		Role:    schema.User,
		Content: "[Max iterations reached]",
	})

	return fmt.Errorf("max iterations (%d) reached", l.maxIterations)
}

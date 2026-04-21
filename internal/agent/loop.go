package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// Loop is the central orchestrator for the execution prompt.
type Loop struct {
	llm           domain.LLM
	toolExecutor  *ToolExecutor
	events        eventSender
	notifier      taskNotifier
	maxIterations int
}

func NewLoop(
	llm domain.LLM,
	toolExecutor *ToolExecutor,
	maxIterations int,
	events eventSender,
	notifier taskNotifier,
) *Loop {
	return &Loop{
		llm:           llm,
		toolExecutor:  toolExecutor,
		events:        events,
		notifier:      notifier,
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
		// 1. Drain background notifications before ANY other work in this turn
		if l.notifier != nil {
			for _, xml := range l.notifier.Drain() {
				session.Messages = append(session.Messages, &schema.Message{
					Role:    schema.User,
					Content: xml,
					Extra:   map[string]any{domain.NotificationMessageExtraKey: true},
				})
			}
		}

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

		// Ambiguity resolution: Ensure every unique ToolID has a stable, unique Index.
		// Some providers (like GitHub Gemini bridge) emit colliding/missing indices for parallel tools.
		// We use a Real-Time Memory Cop to remember which fixed slot each buggy 'original index'
		// currently points to as we process the stream in order.
		idToFixedIdx := make(map[string]int)
		origToFixedIdx := make(map[int]int)
		nextAvailableIdx := 0

		for _, chunk := range chunks {
			for i := range chunk.ToolCalls {
				tc := &chunk.ToolCalls[i]

				// 1. Identify the buggy original index (default 0)
				origIdx := 0
				if tc.Index != nil {
					origIdx = *tc.Index
				}

				if tc.ID != "" {
					// 2. We have an ID. Map it to a stable slot.
					fixedIdx, ok := idToFixedIdx[tc.ID]
					if !ok {
						fixedIdx = nextAvailableIdx
						idToFixedIdx[tc.ID] = fixedIdx
						nextAvailableIdx++
					}

					// 3. Update the chunk and REMEMBER this mapping for this Index
					tc.Index = &fixedIdx
					origToFixedIdx[origIdx] = fixedIdx

				} else if tc.Index != nil {
					// 4. Fragment without ID. Check the Memory Cop for the latest mapping.
					if fixedIdx, ok := origToFixedIdx[origIdx]; ok {
						tc.Index = &fixedIdx
					}
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

		toolRes, err := l.toolExecutor.executeBatch(ctx, msg.ToolCalls, l.events)
		if session.ToolDisplays == nil {
			session.ToolDisplays = make(domain.ToolDisplays)
		}
		maps.Copy(session.ToolDisplays, toolRes.Displays)
		for _, r := range toolRes.Responses {
			if r != nil {
				session.Messages = append(session.Messages, r)
			}
		}
		if err != nil {
			return err
		}
	}

	session.Messages = append(session.Messages, &schema.Message{
		Role:    schema.User,
		Content: "[Max iterations reached]",
	})

	return fmt.Errorf("max iterations (%d) reached", l.maxIterations)
}

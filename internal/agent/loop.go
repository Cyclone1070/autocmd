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

func normalizeToolCallIndices(chunks []*schema.Message) {
	// Ambiguity resolution: Ensure every unique ToolID has a stable, unique Index.
	// Some providers emit colliding/missing indices for parallel tools.
	idToFixedIdx := make(map[string]int)
	origToFixedIdx := make(map[int]int)
	nextAvailableIdx := 0

	for _, chunk := range chunks {
		for i := range chunk.ToolCalls {
			tc := &chunk.ToolCalls[i]

			origIdx := 0
			if tc.Index != nil {
				origIdx = *tc.Index
			}

			if tc.ID != "" {
				fixedIdx, ok := idToFixedIdx[tc.ID]
				if !ok {
					fixedIdx = nextAvailableIdx
					idToFixedIdx[tc.ID] = fixedIdx
					nextAvailableIdx++
				}
				tc.Index = &fixedIdx
				origToFixedIdx[origIdx] = fixedIdx
			} else if tc.Index != nil {
				if fixedIdx, ok := origToFixedIdx[origIdx]; ok {
					tc.Index = &fixedIdx
				}
			}
		}
	}
}

func appendConcatenatedAssistantMessage(session *domain.Session, chunks []*schema.Message) error {
	if len(chunks) == 0 {
		return nil
	}
	normalizeToolCallIndices(chunks)
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return fmt.Errorf("ConcatMessages: %w", err)
	}
	session.Messages = append(session.Messages, msg)
	return nil
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
					Extra:   map[string]any{domain.CancelMessageExtraKey: true},
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
		thinkingSent := false
		for {
			chunk, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if appendErr := appendConcatenatedAssistantMessage(session, chunks); appendErr != nil {
					return appendErr
				}
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
					if !thinkingSent {
						l.events.SendUIUpdate(domain.ThinkingEvent{})
						thinkingSent = true
					}
					l.events.SendUIUpdate(domain.TextEvent{
						Text:      chunk.ReasoningContent,
						IsThought: true,
					})
				}
			}
		}

		if err := appendConcatenatedAssistantMessage(session, chunks); err != nil {
			return err
		}
		if len(chunks) == 0 {
			return nil
		}
		msg := session.Messages[len(session.Messages)-1]

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

package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

// Loop is the central orchestrator for the execution prompt.
type Loop struct {
	toolExecutor  *ToolExecutor
	events        eventSender
	notifier      taskNotifier
	summarizer    *Summarizer
	llm           domain.LLM
	maxIterations int
}

const contextSummaryThreshold = 0.0

func classifyModelErr(err error) error {
	if err == nil {
		return ErrModelBackend
	}
	// Provider SDKs often return transport-specific errors without stable typed surfaces.
	// Normalize common auth/authz markers here so cmd can present a precise message.
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "unauthenticated") ||
		strings.Contains(s, "permission_denied") ||
		strings.Contains(s, "permission denied") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "api key not valid") ||
		strings.Contains(s, "invalid authentication") ||
		strings.Contains(s, "status: unauthenticated") ||
		strings.Contains(s, "status: permission_denied") ||
		strings.Contains(s, "error 401") ||
		strings.Contains(s, "error 403") {
		return ErrModelAuth
	}
	return ErrModelBackend
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

// NewLoop creates a new agent interaction loop.
func NewLoop(
	llm domain.LLM,
	toolExecutor *ToolExecutor,
	maxIterations int,
	events eventSender,
	notifier taskNotifier,
	summarizer *Summarizer,
) *Loop {
	return &Loop{
		llm:           llm,
		toolExecutor:  toolExecutor,
		events:        events,
		notifier:      notifier,
		maxIterations: maxIterations,
		summarizer:    summarizer,
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
		// 1. Context Summary Check
		l.summarizeIfNecessary(ctx, session)

		// 2. Drain background notifications before ANY other work in this turn
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
			return fmt.Errorf("%w: bind tools: %w", classifyModelErr(err), err)
		}

		reader, err := modelWithTools.Stream(ctx, session.Messages)
		if err != nil {
			return fmt.Errorf("%w: LLM.Stream: %w", classifyModelErr(err), err)
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
				return fmt.Errorf("%w: reader.Recv: %w", classifyModelErr(err), err)
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

func (l *Loop) summarizeIfNecessary(ctx context.Context, session *domain.Session) {
	if l.summarizer == nil || session.TotalTokens() <= int(float64(l.llm.ContextWindow())*contextSummaryThreshold) {
		return
	}

	// Summarize history if we are nearing the context limit.
	// We keep the most recent message (the current input) and replace everything before it with a summary.
	if len(session.Messages) > 1 {
		lastMsg := session.Messages[len(session.Messages)-1]
		history := session.Messages[:len(session.Messages)-1]
		summary, err := l.summarizer.Summarize(ctx, history)
		if err == nil {
			// Prompt Stuffing: Instead of creating a new conversational turn, we prepend
			// the summary milestone to the CURRENT user request. This ensures the model
			// sees the context immediately before responding, and avoids Role alternation errors.
			lastMsg.Content = fmt.Sprintf("[Conversation compacted automatically]\n\n%s\n\n=== CURRENT REQUEST ===\n\n%s", summary.Content, lastMsg.Content)
			session.Messages = []*schema.Message{lastMsg}
		}
	}
}

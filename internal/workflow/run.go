package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Run executes the main LLM-tool interaction loop.
func (w *Workflow) Run(ctx context.Context, input string) error {
	// Auto-create session if nil
	if w.currentSession == nil {
		s, err := w.sessionStore.Create()
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		w.currentSession = s
	}

	w.currentSession.Messages = append(w.currentSession.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: input,
	})

	defer func() {
		if w.events != nil {
			select {
			case w.events <- domain.DoneEvent{}:
			default:
			}
		}
	}()

	maxIterations := w.cfg.Tools.MaxIterations
	for range maxIterations {
		if err := ctx.Err(); err != nil {
			w.currentSession.Messages = append(w.currentSession.Messages, domain.Message{
				Role:    domain.RoleUser,
				Content: "[Session cancelled by user]",
			})
			_ = w.sessionStore.Save(w.currentSession)
			return err
		}

		if w.events != nil {
			w.events <- domain.ThinkingEvent{}
		}

		if w.currentModel == nil {
			return fmt.Errorf("no model selected")
		}

		stream, err := w.currentModel.Stream(ctx, w.currentSession.Messages, w.toolExecutor.declarations())
		if err != nil {
			_ = w.sessionStore.Save(w.currentSession)
			return fmt.Errorf("model.Stream: %w", err)
		}

		var msg domain.Message
		msg.Role = domain.RoleAssistant

		for stream.Next() {
			switch c := stream.Chunk().(type) {
			case domain.TextChunk:
				msg.Content += c.Text
				if w.events != nil {
					w.events <- domain.TextEvent{Text: c.Text}
				}
			case domain.ToolCall:
				msg.ToolCalls = append(msg.ToolCalls, c)
			}
		}

		if err := stream.Err(); err != nil {
			_ = w.sessionStore.Save(w.currentSession)
			return fmt.Errorf("stream.Err: %w", err)
		}

		w.currentSession.Messages = append(w.currentSession.Messages, msg)

		if len(msg.ToolCalls) == 0 {
			_ = w.sessionStore.Save(w.currentSession)
			return nil
		}

		for _, tc := range msg.ToolCalls {
			toolResp, err := w.toolExecutor.execute(ctx, tc, w.events)
			if err != nil {
				_ = w.sessionStore.Save(w.currentSession)
				return fmt.Errorf("tools.Execute (%s): %w", tc.Name, err)
			}
			w.currentSession.Messages = append(w.currentSession.Messages, toolResp)
		}
	}

	w.currentSession.Messages = append(w.currentSession.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: "[Max iterations reached]",
	})

	_ = w.sessionStore.Save(w.currentSession)
	return fmt.Errorf("max iterations (%d) reached", maxIterations)
}

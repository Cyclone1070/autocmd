package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Run executes the main LLM-tool interaction loop.
func (w *Workflow) Run(ctx context.Context, input string) error {
	// Create cancellable child context for internal cancellation
	runCtx, runCancel := context.WithCancel(ctx)
	runDone := make(chan struct{})

	w.mu.Lock()
	w.runCancel = runCancel
	w.runDone = runDone
	w.mu.Unlock()

	defer func() {
		close(runDone)
		w.mu.Lock()
		w.runCancel = nil
		w.runDone = nil
		w.mu.Unlock()
	}()

	// Auto-create session if nil
	if w.currentSession == nil {
		s, err := w.sessionStore.Create()
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		w.currentSession = s
	}

	// Capture session and model at start to avoid race with SwitchSession/SetModel
	sess := w.currentSession
	model := w.currentModel

	sess.Messages = append(sess.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: input,
	})

	defer func() {
		if w.events != nil {
			// Non-blocking send to prevent deadlock if channel is full.
			// This defer must complete so runDone can be closed.
			select {
			case w.events <- DoneEvent{}:
			default:
			}
		}
	}()

	maxIterations := w.cfg.Tools.MaxIterations
	for range maxIterations {
		if err := runCtx.Err(); err != nil {
			sess.Messages = append(sess.Messages, domain.Message{
				Role:    domain.RoleUser,
				Content: "[Session cancelled by user]",
			})
			_ = w.sessionStore.Save(sess)
			return err
		}

		if w.events != nil {
			w.events <- ThinkingEvent{}
		}

		if w.currentProvider == nil {
			return fmt.Errorf("no provider selected")
		}

		stream, err := w.currentProvider.Stream(runCtx, model, sess.Messages, w.toolExecutor.declarations())
		if err != nil {
			_ = w.sessionStore.Save(sess)
			return fmt.Errorf("provider.Stream: %w", err)
		}

		var msg domain.Message
		msg.Role = domain.RoleAssistant

		for stream.Next() {
			switch c := stream.Chunk().(type) {
			case domain.TextChunk:
				msg.Content += c.Text
				if w.events != nil {
					w.events <- TextEvent{Text: c.Text}
				}
			case domain.ToolCall:
				msg.ToolCalls = append(msg.ToolCalls, c)
			}
		}

		if err := stream.Err(); err != nil {
			_ = w.sessionStore.Save(sess)
			return fmt.Errorf("stream.Err: %w", err)
		}

		sess.Messages = append(sess.Messages, msg)

		if len(msg.ToolCalls) == 0 {
			_ = w.sessionStore.Save(sess)
			return nil
		}

		for _, tc := range msg.ToolCalls {
			toolResp, err := w.toolExecutor.execute(runCtx, tc, w.events)
			if err != nil {
				_ = w.sessionStore.Save(sess)
				return fmt.Errorf("tools.Execute (%s): %w", tc.Name, err)
			}
			sess.Messages = append(sess.Messages, toolResp)
		}

	}

	sess.Messages = append(sess.Messages, domain.Message{
		Role:    domain.RoleUser,
		Content: "[Max iterations reached]",
	})

	_ = w.sessionStore.Save(sess)
	return fmt.Errorf("max iterations (%d) reached", maxIterations)
}

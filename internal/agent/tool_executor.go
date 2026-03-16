package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

type toolExecutor struct {
	registry toolRegistry
}

func newToolExecutor(registry toolRegistry) *toolExecutor {
	return &toolExecutor{
		registry: registry,
	}
}

func (e *toolExecutor) declarations() []domain.Declaration {
	return e.registry.Declarations()
}

func (e *toolExecutor) execute(ctx context.Context, tc domain.ToolCall, events eventSender) (domain.ToolMessage, domain.ToolDisplay, error) {
	t, ok := e.registry.Get(tc.Name)
	if !ok {
		decls := e.declarations()
		declsJSON, jerr := json.MarshalIndent(decls, "", "  ")
		if jerr != nil {
			slog.Warn("Failed to marshal tool declarations for LLM prompt", "err", jerr)
		}
		errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Name, declsJSON)

		return domain.ToolMessage{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    errMsg,
		}, nil, nil
	}

	inv, err := t.Prepare(ctx, tc.Arguments)
	if err != nil {
		if ctx.Err() != nil {
			return domain.ToolMessage{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    "execution cancelled",
				ToolError:  true,
			}, nil, ctx.Err()
		}
		declJSON, jerr := json.MarshalIndent(t.Declaration(), "", "  ")
		if jerr != nil {
			slog.Warn("Failed to marshal tool declaration", "tool", t.Name(), "err", jerr)
		}
		errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Name, err, declJSON)

		return domain.ToolMessage{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    errMsg,
		}, nil, nil
	}

	display := inv.Display()

	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Name,
			Display:  display,
		})
	}

	// We no longer wait for streaming to complete before returning from execute.
	// The broker/Loop will handle the async nature of streaming.

	var streamWG sync.WaitGroup
	if sh, ok := display.(domain.ShellDisplay); ok && sh.Output != nil && events != nil {
		streamWG.Add(1)
		go func() {
			defer streamWG.Done()
			buf := make([]byte, 1024*1024)
			for {
				n, err := sh.Output.Read(buf)
				if n > 0 {
					events.SendUIUpdate(domain.ToolStreamEvent{
						CallID: tc.ID,
						Chunk:  string(buf[:n]),
					})
				}
				if err != nil {
					break
				}
			}

		}()
	}

	llmContent, err := inv.Execute(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return domain.ToolMessage{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    "execution cancelled",
				ToolError:  true,
			}, display, err
		}

		if events != nil {
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID: tc.ID,
				Error:  "Execution failed",
			})
		}

		return domain.ToolMessage{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    llmContent,
			ToolError:  true,
		}, display, nil
	}

	// Ensure all streaming chunks are sent before sending the end event
	streamWG.Wait()

	// Always send the end event after Execute returns (regardless of tool type)
	if events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID: tc.ID,
		})
	}

	return domain.ToolMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    llmContent,
	}, display, nil
}

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

type toolExecutor struct {
	registry toolRegistry
}

func newToolExecutor(registry toolRegistry) *toolExecutor {
	return &toolExecutor{
		registry: registry,
	}
}

func (e *toolExecutor) definitions() []*schema.ToolInfo {
	return e.registry.Definitions()
}

func (e *toolExecutor) execute(ctx context.Context, tc *schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay, error) {
	t, ok := e.registry.Get(tc.Function.Name)
	if !ok {
		defs := e.definitions()
		defsJSON, jerr := json.MarshalIndent(defs, "", "  ")
		if jerr != nil {
			slog.Warn("Failed to marshal tool definitions for LLM prompt", "err", jerr)
		}
		errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Function.Name, defsJSON)

		display := domain.NewStringDisplay("", "Unknown tool")
		if events != nil {
			events.SendUIUpdate(domain.ToolStartEvent{
				CallID:   tc.ID,
				ToolName: tc.Function.Name,
				Display:  display,
			})
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID: tc.ID,
				Error:  "Unknown tool",
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    errMsg,
			Extra:      map[string]any{"tool_error": true},
		}, display, nil
	}

	inv, err := t.Prepare(ctx, tc.Function.Arguments)
	if err != nil {
		if ctx.Err() != nil {
			return &schema.Message{
				Role:       schema.Tool,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    "execution cancelled",
				Extra:      map[string]any{"tool_error": true},
			}, nil, ctx.Err()
		}
		defJSON, jerr := json.MarshalIndent(t.Definition(), "", "  ")
		if jerr != nil {
			slog.Warn("Failed to marshal tool definition", "tool", t.Name(), "err", jerr)
		}
		errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Function.Name, err, defJSON)

		toolLabel := fmt.Sprintf("Bad %s request", strings.ToUpper(strings.ReplaceAll(tc.Function.Name, "_", " ")))
		display := domain.NewStringDisplay("", toolLabel)
		if events != nil {
			events.SendUIUpdate(domain.ToolStartEvent{
				CallID:   tc.ID,
				ToolName: tc.Function.Name,
				Display:  display,
			})
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID: tc.ID,
				Error:  toolLabel,
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    errMsg,
			Extra:      map[string]any{"tool_error": true},
		}, display, nil
	}

	display := inv.Display()

	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Function.Name,
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
			return &schema.Message{
				Role:       schema.Tool,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    "execution cancelled",
				Extra:      map[string]any{"tool_error": true},
			}, display, ctx.Err()
		}

		errStr := err.Error()
		switch d := display.(type) {
		case domain.StringDisplay:
			d.Error = errStr
			display = d
		case domain.DiffDisplay:
			d.Error = errStr
			display = d
		case domain.ShellDisplay:
			d.Error = errStr
			display = d
		}

		if events != nil {
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID: tc.ID,
				Error:  errStr,
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    llmContent,
			Extra:      map[string]any{"tool_error": true},
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

	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    llmContent,
	}, display, nil
}

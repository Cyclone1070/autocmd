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

		display := domain.NewStringDisplay("", "Tool call failed")
		endDisp := display
		endDisp.Error = "Unknown tool"
		if events != nil {
			events.SendUIUpdate(domain.ToolStartEvent{
				CallID:   tc.ID,
				ToolName: tc.Function.Name,
				Display:  display,
			})
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID:  tc.ID,
				Display: endDisp,
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    errMsg,
		}, endDisp, nil
	}

	inv, err := t.Prepare(tc.Function.Arguments)
	if err != nil {
		defJSON, jerr := json.MarshalIndent(t.Definition(), "", "  ")
		if jerr != nil {
			slog.Warn("Failed to marshal tool definition", "tool", t.Name(), "err", jerr)
		}
		errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Function.Name, err, defJSON)

		toolLabel := fmt.Sprintf("Bad %s request", strings.ToUpper(strings.ReplaceAll(tc.Function.Name, "_", " ")))
		display := domain.NewStringDisplay("", "Tool call failed")
		endDisp := display
		endDisp.Error = toolLabel
		if events != nil {
			events.SendUIUpdate(domain.ToolStartEvent{
				CallID:   tc.ID,
				ToolName: tc.Function.Name,
				Display:  display,
			})
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID:  tc.ID,
				Display: endDisp,
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    errMsg,
		}, endDisp, nil
	}

	previewDisplay := inv.Display()

	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Function.Name,
			Display:  previewDisplay,
		})
	}

	// We no longer wait for streaming to complete before returning from execute.
	// The broker/Loop will handle the async nature of streaming.

	var streamWG sync.WaitGroup
	if si, ok := inv.(domain.StreamableInvocation); ok && events != nil {
		stream := si.Stream()
		if stream != nil {
			streamWG.Add(1)
			go func() {
				defer streamWG.Done()
				buf := make([]byte, 1024*1024)
				for {
					n, err := stream.Read(buf)
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
	}

	llmContent, finalDisplay, err := inv.Execute(ctx)
	if finalDisplay == nil {
		panic(fmt.Sprintf("tool %q Execute returned nil finalDisplay (callID=%s)", tc.Function.Name, tc.ID))
	}

	if err != nil {
		if ctx.Err() != nil {
			if events != nil {
				events.SendUIUpdate(domain.ToolEndEvent{
					CallID:  tc.ID,
					Display: finalDisplay,
				})
			}
			return &schema.Message{
				Role:       schema.Tool,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    "execution cancelled",
			}, finalDisplay, ctx.Err()
		}

		if events != nil {
			events.SendUIUpdate(domain.ToolEndEvent{
				CallID:  tc.ID,
				Display: finalDisplay,
			})
		}

		return &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    llmContent,
		}, finalDisplay, nil
	}

	// Ensure all streaming chunks are sent before sending the end event
	streamWG.Wait()

	// Always send the end event after Execute returns (regardless of tool type)
	if events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tc.ID,
			Display: finalDisplay,
		})
	}

	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    llmContent,
	}, finalDisplay, nil
}

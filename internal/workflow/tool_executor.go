package workflow

import (
	"context"
	"encoding/json"
	"fmt"
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

func (e *toolExecutor) execute(ctx context.Context, tc domain.ToolCall, events chan<- domain.Event) (domain.Message, error) {
	t, ok := e.registry.Get(tc.Name)
	if !ok {
		decls := e.declarations()
		declsJSON, _ := json.MarshalIndent(decls, "", "  ")
		errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Name, declsJSON)

		return domain.Message{
			Role:       domain.RoleTool,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    errMsg,
		}, nil
	}

	inv, err := t.Prepare(ctx, tc.Arguments)
	if err != nil {
		if ctx.Err() != nil {
			return domain.Message{}, ctx.Err()
		}
		declJSON, _ := json.MarshalIndent(t.Declaration(), "", "  ")
		errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Name, err, declJSON)

		return domain.Message{
			Role:       domain.RoleTool,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    errMsg,
		}, nil
	}

	display := inv.Display()

	if events != nil {
		events <- domain.ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Name,
			Display:  display,
		}
	}

	var streamWg sync.WaitGroup
	// Defer ensures goroutine cleanup on ALL paths (including early error returns).
	// The explicit Wait() below ensures streaming completes before ToolEndEvent.
	defer streamWg.Wait()

	if sh, ok := display.(domain.ShellDisplay); ok && sh.Output != nil && events != nil {
		streamWg.Go(func() {
			buf := make([]byte, 4096)
			for {
				n, err := sh.Output.Read(buf)
				if n > 0 {
					events <- domain.ToolStreamEvent{
						CallID: tc.ID,
						Chunk:  string(buf[:n]),
					}
				}
				if err != nil {
					break
				}
			}
			// NOTE: Do NOT call sh.Wait() here - Execute() already calls streamCmd.Wait()
			// Calling it twice causes a race condition.
		})
	}

	llmContent, err := inv.Execute(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return domain.Message{}, err
		}

		if events != nil {
			events <- domain.ToolEndEvent{
				CallID: tc.ID,
				Error:  "Execution failed",
			}
		}

		return domain.Message{
			Role:       domain.RoleTool,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    llmContent,
		}, nil
	}

	// Wait for streaming goroutine to finish reading all output
	streamWg.Wait()

	if events != nil {
		events <- domain.ToolEndEvent{
			CallID: tc.ID,
		}
	}

	return domain.Message{
		Role:       domain.RoleTool,
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    llmContent,
	}, nil
}

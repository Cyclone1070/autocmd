package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/tool"
)

type toolManager struct {
	registry map[string]Tool
}

func newToolManager(tools []Tool) *toolManager {
	tm := &toolManager{
		registry: make(map[string]Tool),
	}
	for _, t := range tools {
		tm.registry[t.Name()] = t
	}
	return tm
}

func (m *toolManager) declarations() []tool.Declaration {
	decls := make([]tool.Declaration, 0, len(m.registry))
	for _, t := range m.registry {
		decls = append(decls, t.Declaration())
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

func (m *toolManager) execute(ctx context.Context, tc provider.ToolCall, events chan<- Event) (provider.Message, error) {
	t, ok := m.registry[tc.Function.Name]
	if !ok {
		decls := m.declarations()
		declsJSON, _ := json.MarshalIndent(decls, "", "  ")
		errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Function.Name, declsJSON)

		return provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: tc.ID,
			Content:    errMsg,
		}, nil
	}

	inv, err := t.Prepare(ctx, tc.Function.Arguments)
	if err != nil {
		if ctx.Err() != nil {
			return provider.Message{}, ctx.Err()
		}
		declJSON, _ := json.MarshalIndent(t.Declaration(), "", "  ")
		errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Function.Name, err, declJSON)

		return provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: tc.ID,
			Content:    errMsg,
		}, nil
	}

	display := inv.Display()

	if events != nil {
		events <- ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Function.Name,
			Display:  display,
		}
	}

	var streamWg sync.WaitGroup
	// Defer ensures goroutine cleanup on ALL paths (including early error returns).
	// The explicit Wait() below ensures streaming completes before ToolEndEvent.
	defer streamWg.Wait()

	if sh, ok := display.(tool.ShellDisplay); ok && sh.Output != nil && events != nil {
		streamWg.Add(1)
		go func() {
			defer streamWg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := sh.Output.Read(buf)
				if n > 0 {
					events <- ToolStreamEvent{
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
		}()
	}

	llmContent, err := inv.Execute(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return provider.Message{}, err
		}

		if events != nil {
			events <- ToolEndEvent{
				CallID: tc.ID,
				Error:  "Execution failed",
			}
		}

		return provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: tc.ID,
			Content:    llmContent,
		}, nil
	}

	// Wait for streaming goroutine to finish reading all output
	streamWg.Wait()

	if events != nil {
		events <- ToolEndEvent{
			CallID: tc.ID,
		}
	}

	return provider.Message{
		Role:       provider.RoleTool,
		ToolCallID: tc.ID,
		Content:    llmContent,
	}, nil
}

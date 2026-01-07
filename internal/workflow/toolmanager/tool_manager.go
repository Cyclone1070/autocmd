package toolmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/workflow"
)

type ToolManager struct {
	registry map[string]Tool
}

func NewToolManager(tools ...Tool) *ToolManager {
	tm := &ToolManager{
		registry: make(map[string]Tool),
	}
	for _, t := range tools {
		tm.Register(t)
	}
	return tm
}

func (m *ToolManager) Register(t Tool) {
	m.registry[t.Name()] = t
}

func (m *ToolManager) Declarations() []tool.Declaration {
	decls := make([]tool.Declaration, 0, len(m.registry))
	for _, t := range m.registry {
		decls = append(decls, t.Declaration())
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

func (m *ToolManager) Execute(ctx context.Context, tc provider.ToolCall, events chan<- workflow.Event) (provider.Message, error) {
	t, ok := m.registry[tc.Function.Name]
	if !ok {
		decls := m.Declarations()
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
		events <- workflow.ToolStartEvent{
			CallID:   tc.ID,
			ToolName: tc.Function.Name,
			Display:  display,
		}
	}

	var streamWg sync.WaitGroup
	// Special handling for shell streaming: start reading from the pipe BEFORE Execute()
	if sh, ok := display.(tool.ShellDisplay); ok && sh.Output != nil && events != nil {
		streamWg.Add(1)
		go func() {
			defer streamWg.Done()
			buf := make([]byte, 4096)
			for {
				n, err := sh.Output.Read(buf)
				if n > 0 {
					events <- workflow.ToolStreamEvent{
						CallID: tc.ID,
						Chunk:  string(buf[:n]),
					}
				}
				if err != nil {
					break
				}
			}
			if sh.Wait != nil {
				sh.Wait()
			}
		}()
	}

	llmContent, err := inv.Execute(ctx)
	if err != nil {
		// Per contract, tools only return errors for infrastructure issues (context cancellation)
		if ctx.Err() != nil {
			return provider.Message{}, err
		}

		// Execution failed (e.g. write failure), but loop continues safely
		if events != nil {
			events <- workflow.ToolEndEvent{
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

	streamWg.Wait()
	if events != nil {
		events <- workflow.ToolEndEvent{
			CallID: tc.ID,
		}
	}

	return provider.Message{
		Role:       provider.RoleTool,
		ToolCallID: tc.ID,
		Content:    llmContent,
	}, nil
}

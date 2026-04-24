package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/permission"
	"github.com/cloudwego/eino/schema"
)

type ToolExecutor struct {
	registry           toolRegistry
	waiter             actionWaiter
	permissionResolver *permission.Resolver
}

type batchResult struct {
	Responses []*schema.Message
	Displays  map[string]domain.ToolDisplay
}

// NewToolExecutor creates a new ToolExecutor with its dependencies.
func NewToolExecutor(registry toolRegistry, waiter actionWaiter, permissionResolver ...*permission.Resolver) *ToolExecutor {
	var resolver *permission.Resolver
	if len(permissionResolver) > 0 {
		resolver = permissionResolver[0]
	}
	return &ToolExecutor{
		registry:           registry,
		waiter:             waiter,
		permissionResolver: resolver,
	}
}

func (e *ToolExecutor) definitions() []*schema.ToolInfo {
	return e.registry.Definitions()
}

func (e *ToolExecutor) executeBatch(ctx context.Context, calls []schema.ToolCall, events eventSender) (batchResult, error) {
	responses := make([]*schema.Message, len(calls))
	displays := make(map[string]domain.ToolDisplay)

	for i := 0; i < len(calls); {
		tc := calls[i]
		tool, _ := e.registry.Get(tc.Function.Name)

		// 1. Barrier check (unknown or non-concurrent-safe tools)
		if tool == nil || !tool.IsConcurrentSafe() {
			inv, msg, disp := e.prepareTool(ctx, &tc, events)
			// No inv means execution stops here
			if inv == nil {
				responses[i] = msg
				displays[tc.ID] = disp
			} else {
				resp, finalDisp := e.executeTool(ctx, &tc, inv, events)
				responses[i] = resp
				displays[tc.ID] = finalDisp
			}
			i++
			continue
		}

		// 2. Greedy batching for concurrent-safe tools
		start := i
		for i < len(calls) {
			tNext, _ := e.registry.Get(calls[i].Function.Name)
			if tNext == nil || !tNext.IsConcurrentSafe() {
				break
			}
			i++
		}
		segment := calls[start:i]

		// Prepare all in segment sequentially to ensure StartEvents are in input order
		preparedInvs := make([]domain.Invocation, len(segment))
		failureOutcomes := make([]*struct {
			resp *schema.Message
			disp domain.ToolDisplay
		}, len(segment))

		for j := range segment {
			inv, msg, disp := e.prepareTool(ctx, &segment[j], events)
			if inv == nil {
				failureOutcomes[j] = &struct {
					resp *schema.Message
					disp domain.ToolDisplay
				}{msg, disp}
			} else {
				preparedInvs[j] = inv
			}
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		for j, tcBatch := range segment {
			if failureOutcomes[j] != nil {
				responses[start+j] = failureOutcomes[j].resp
				if failureOutcomes[j].disp != nil {
					displays[tcBatch.ID] = failureOutcomes[j].disp
				}
				continue
			}

			wg.Add(1)
			go func(idx int, tcb schema.ToolCall, inv domain.Invocation) {
				defer wg.Done()
				resp, disp := e.executeTool(ctx, &tcb, inv, events)
				mu.Lock()
				defer mu.Unlock()
				responses[start+idx] = resp
				if disp != nil {
					displays[tcb.ID] = disp
				}
			}(j, tcBatch, preparedInvs[j])
		}
		wg.Wait()
	}

	return batchResult{
		Responses: responses,
		Displays:  displays,
	}, ctx.Err()
}

func emitToolStart(events eventSender, callID string, display domain.ToolDisplay) {
	if events != nil && display != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  callID,
			Display: display,
		})
	}
}

func (e *ToolExecutor) prepareTool(ctx context.Context, tc *schema.ToolCall, events eventSender) (domain.Invocation, *schema.Message, domain.ToolDisplay) {
	toolName := tc.Function.Name
	callID := tc.ID

	if ctx.Err() != nil {
		return nil, &schema.Message{
			Role:       schema.Tool,
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    domain.ToolErrorCancelled,
		}, nil
	}

	tool, ok := e.registry.Get(toolName)
	if !ok {
		msg, disp := e.unknownToolOutcome(tc, events)
		return nil, msg, disp
	}
	if e.permissionResolver.Resolve(toolName) == permission.ModeDeny {
		msg, disp := e.permissionDeniedBeforePrepareOutcome(tc, events)
		return nil, msg, disp
	}

	inv, err := tool.Prepare(tc.Function.Arguments)
	if err != nil {
		msg, disp := e.prepareFailureOutcome(tool, tc, err, events)
		return nil, msg, disp
	}

	previewDisplay := inv.Display()
	if previewDisplay == nil {
		panic(fmt.Sprintf("tool %q Prepare returned invocation with nil display (callID=%s)", toolName, callID))
	}
	if _, isInteractive := inv.(domain.InteractiveInvocation); isInteractive {
		emitToolStart(events, callID, previewDisplay)
		return inv, nil, nil
	}
	if e.permissionResolver.Resolve(toolName) == permission.ModeAsk {
		emitToolStart(events, callID, previewDisplay)
		if events != nil {
			events.SendUIUpdate(domain.ToolApprovalRequestEvent{CallID: callID})
		}
		approved := e.waitForPermissionDecision(ctx, callID)
		if !approved {
			msg, disp := e.permissionDeniedAfterPrepareOutcome(tc, previewDisplay, events)
			return nil, msg, disp
		}
		return inv, nil, nil
	}

	emitToolStart(events, callID, previewDisplay)
	return inv, nil, nil
}

func (e *ToolExecutor) executeTool(ctx context.Context, tc *schema.ToolCall, inv domain.Invocation, events eventSender) (*schema.Message, domain.ToolDisplay) {
	toolName := tc.Function.Name
	callID := tc.ID

	// Identification & Execution
	var llmContent string
	var finalDisplay domain.ToolDisplay

	if interInv, ok := inv.(domain.InteractiveInvocation); ok {
		// Wait for action (interactive path)
		if e.waiter == nil {
			panic("toolExecutor.executeTool: InteractiveInvocation encountered but no actionWaiter provided")
		}

		action, _ := e.waiter.Wait(ctx, callID)

		// On success or user cancellation (ctx.Err() != nil), we ask the tool to resolve the final state.
		llmContent, finalDisplay = interInv.Resolve(ctx, action)
	} else if execInv, ok := inv.(domain.ExecutableInvocation); ok {
		// Standard synchronous execution
		var si domain.StreamableInvocation
		if s, sOk := inv.(domain.StreamableInvocation); sOk {
			si = s
		}

		// Handle streaming UI output if applicable
		var streamWG sync.WaitGroup
		if si != nil && events != nil {
			stream := si.Stream()
			if stream != nil {
				streamWG.Go(func() {
					buf := make([]byte, 1024*1024)
					for {
						n, readErr := stream.Read(buf)
						if n > 0 {
							events.SendUIUpdate(domain.ToolStreamEvent{
								CallID: callID,
								Chunk:  string(buf[:n]),
							})
						}
						if readErr != nil {
							break
						}
					}
				})
			}
		}

		llmContent, finalDisplay = execInv.Execute(ctx)
		streamWG.Wait()
	} else {
		panic(fmt.Sprintf("tool %q invocation implements neither Executable nor Interactive", toolName))
	}

	if finalDisplay == nil {
		panic(fmt.Sprintf("tool %q returned nil finalDisplay (callID=%s)", toolName, callID))
	}

	// Final events and response
	if events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  callID,
			Display: finalDisplay,
		})
	}

	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: callID,
		ToolName:   toolName,
		Content:    llmContent,
	}, finalDisplay
}

func (e *ToolExecutor) unknownToolOutcome(tc *schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay) {
	defs := e.definitions()
	defsJSON, _ := json.MarshalIndent(defs, "", "  ")
	errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Function.Name, defsJSON)

	display := domain.NewStringDisplay("", "")
	display.Error = "Unknown tool"
	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  tc.ID,
			Display: display,
		})
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tc.ID,
			Display: display,
		})
	}

	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    errMsg,
	}, display
}

func (e *ToolExecutor) permissionDeniedMessage(tc *schema.ToolCall) *schema.Message {
	errMsg := fmt.Sprintf("Error: permission denied for %q tool", tc.Function.Name)
	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    errMsg,
	}
}

func (e *ToolExecutor) permissionDeniedBeforePrepareOutcome(tc *schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay) {
	display := domain.NewStringDisplay("", "").WithError(domain.ToolErrorPermissionDenied)
	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  tc.ID,
			Display: display,
		})
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tc.ID,
			Display: display,
		})
	}
	return e.permissionDeniedMessage(tc), display
}

func (e *ToolExecutor) permissionDeniedAfterPrepareOutcome(tc *schema.ToolCall, previewDisplay domain.ToolDisplay, events eventSender) (*schema.Message, domain.ToolDisplay) {
	display := previewDisplay.WithError(domain.ToolErrorPermissionDenied)
	if events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tc.ID,
			Display: display,
		})
	}
	return e.permissionDeniedMessage(tc), display
}

func (e *ToolExecutor) waitForPermissionDecision(ctx context.Context, callID string) bool {
	if e.waiter == nil {
		return false
	}
	for {
		action, err := e.waiter.Wait(ctx, callID)
		if err != nil {
			return false
		}
		decision, ok := action.(domain.PermissionDecisionAction)
		if !ok {
			continue
		}
		return decision.Approved
	}
}

func (e *ToolExecutor) prepareFailureOutcome(t domain.Tool, tc *schema.ToolCall, prepErr error, events eventSender) (*schema.Message, domain.ToolDisplay) {
	defJSON, _ := json.MarshalIndent(t.Definition(), "", "  ")
	errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Function.Name, prepErr, defJSON)

	toolLabel := fmt.Sprintf("Bad %s request", strings.ToUpper(strings.ReplaceAll(tc.Function.Name, "_", " ")))
	display := domain.NewStringDisplay("", "")
	display.Error = toolLabel
	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  tc.ID,
			Display: display,
		})
		events.SendUIUpdate(domain.ToolEndEvent{
			CallID:  tc.ID,
			Display: display,
		})
	}

	return &schema.Message{
		Role:       schema.Tool,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    errMsg,
	}, display
}

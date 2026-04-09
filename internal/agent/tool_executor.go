package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

type ToolExecutor struct {
	registry toolRegistry
	waiter   actionWaiter
}

type batchResult struct {
	Responses []*schema.Message
	Displays  map[string]domain.ToolDisplay
}

// NewToolExecutor creates a new ToolExecutor with its dependencies.
func NewToolExecutor(registry toolRegistry, waiter actionWaiter) *ToolExecutor {
	return &ToolExecutor{
		registry: registry,
		waiter:   waiter,
	}
}

func (e *ToolExecutor) definitions() []*schema.ToolInfo {
	return e.registry.Definitions()
}

type batchOutcome struct {
	index   int
	callID  string
	resp    *schema.Message
	display domain.ToolDisplay
}

type preparedCall struct {
	index          int
	callID         string
	toolName       string
	inv            domain.Invocation
	previewDisplay domain.ToolDisplay
	concurrentSafe bool
}

func emitToolStart(events eventSender, callID string, previewDisplay domain.ToolDisplay) {
	if events == nil {
		return
	}
	events.SendUIUpdate(domain.ToolStartEvent{
		CallID:  callID,
		Display: previewDisplay,
	})
}

func bakeCancelledOutcomeFromPreview(p preparedCall, fatalErr error) batchOutcome {
	content := "tool failed unexpectedly while executing"
	displayErr := "Infrastructure failed"
	if errors.Is(fatalErr, context.Canceled) {
		content = "execution cancelled"
		displayErr = domain.ToolErrorCancelled
	}
	display := p.previewDisplay.WithError(displayErr)
	msg := &schema.Message{
		Role:       schema.Tool,
		ToolCallID: p.callID,
		ToolName:   p.toolName,
		Content:    content,
	}
	return batchOutcome{
		index:   p.index,
		callID:  p.callID,
		resp:    msg,
		display: display,
	}
}

func (e *ToolExecutor) executeBatch(ctx context.Context, calls []schema.ToolCall, events eventSender) (batchResult, error) {
	outcomes := make([]batchOutcome, len(calls))
	plans := make([]preparedCall, 0, len(calls))
	for i, tc := range calls {
		plan, out := e.preflightCall(i, tc, events)
		outcomes[i] = out
		if plan != nil {
			plans = append(plans, *plan)
		}
	}

	var fatalErr error
	for idx := 0; idx < len(plans); {
		if !plans[idx].concurrentSafe {
			emitToolStart(events, plans[idx].callID, plans[idx].previewDisplay)
			resp, disp, err := e.executeInvocation(ctx, plans[idx].callID, plans[idx].toolName, plans[idx].inv, events)
			outcomes[plans[idx].index] = batchOutcome{
				index:   plans[idx].index,
				callID:  plans[idx].callID,
				resp:    resp,
				display: disp,
			}
			if err != nil {
				fatalErr = err
				break
			}
			idx++
			continue
		}

		start := idx
		for idx < len(plans) && plans[idx].concurrentSafe {
			idx++
		}
		segment := plans[start:idx]
		for _, p := range segment {
			emitToolStart(events, p.callID, p.previewDisplay)
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		var firstErr error
		for _, p := range segment {
			wg.Add(1)
			go func(p preparedCall) {
				defer wg.Done()
				resp, disp, err := e.executeInvocation(ctx, p.callID, p.toolName, p.inv, events)
				mu.Lock()
				defer mu.Unlock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				outcomes[p.index] = batchOutcome{
					index:   p.index,
					callID:  p.callID,
					resp:    resp,
					display: disp,
				}
			}(p)
		}
		wg.Wait()
		if firstErr != nil {
			fatalErr = firstErr
			break
		}
	}

	// Fatal errors (infra/cancellation) must still produce tool-result messages for
	// all calls that were preflighted successfully but never got executed.
	if fatalErr != nil {
		for _, p := range plans {
			if outcomes[p.index].resp == nil {
				outcomes[p.index] = bakeCancelledOutcomeFromPreview(p, fatalErr)
			}
		}
	}

	res := batchResult{
		Responses: make([]*schema.Message, len(outcomes)),
		Displays:  make(map[string]domain.ToolDisplay, len(outcomes)),
	}
	for i, out := range outcomes {
		res.Responses[i] = out.resp
		if out.display != nil && out.callID != "" {
			res.Displays[out.callID] = out.display
		}
	}
	return res, fatalErr
}

func (e *ToolExecutor) preflightCall(index int, tc schema.ToolCall, events eventSender) (*preparedCall, batchOutcome) {
	t, ok := e.registry.Get(tc.Function.Name)
	if !ok {
		resp, disp := e.unknownToolOutcome(tc, events)
		return nil, batchOutcome{index: index, callID: tc.ID, resp: resp, display: disp}
	}

	inv, err := t.Prepare(tc.Function.Arguments)
	if err != nil {
		resp, disp := e.prepareFailureOutcome(t, tc, err, events)
		return nil, batchOutcome{index: index, callID: tc.ID, resp: resp, display: disp}
	}
	previewDisplay := inv.Display()
	if previewDisplay == nil {
		panic(fmt.Sprintf("tool %q Prepare returned invocation with nil display (callID=%s)", tc.Function.Name, tc.ID))
	}

	return &preparedCall{
		index:          index,
		callID:         tc.ID,
		toolName:       tc.Function.Name,
		inv:            inv,
		previewDisplay: previewDisplay,
		concurrentSafe: t.IsConcurrentSafe(),
	}, batchOutcome{index: index, callID: tc.ID}
}

func (e *ToolExecutor) unknownToolOutcome(tc schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay) {
	defs := e.definitions()
	defsJSON, _ := json.MarshalIndent(defs, "", "  ")
	errMsg := fmt.Sprintf("Error: tool %q does not exist.\n\nAvailable tools:\n%s", tc.Function.Name, defsJSON)

	display := domain.NewStringDisplay("", "Tool call failed")
	endDisp := display.WithError("Unknown tool")
	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  tc.ID,
			Display: display,
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
	}, endDisp
}

func (e *ToolExecutor) prepareFailureOutcome(t domain.Tool, tc schema.ToolCall, prepErr error, events eventSender) (*schema.Message, domain.ToolDisplay) {
	defJSON, _ := json.MarshalIndent(t.Definition(), "", "  ")
	errMsg := fmt.Sprintf("Error: failed to prepare tool %q: %v\n\nExpected schema:\n%s", tc.Function.Name, prepErr, defJSON)

	toolLabel := fmt.Sprintf("Bad %s request", strings.ToUpper(strings.ReplaceAll(tc.Function.Name, "_", " ")))
	display := domain.NewStringDisplay("", "Tool call failed")
	endDisp := display.WithError(toolLabel)
	if events != nil {
		events.SendUIUpdate(domain.ToolStartEvent{
			CallID:  tc.ID,
			Display: display,
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
	}, endDisp
}

func (e *ToolExecutor) executeInvocation(
	ctx context.Context,
	callID string,
	toolName string,
	inv domain.Invocation,
	events eventSender,
) (*schema.Message, domain.ToolDisplay, error) {
	execInv, ok := inv.(domain.ExecutableInvocation)
	if !ok {
		interInv, ok := inv.(domain.InteractiveInvocation)
		if !ok {
			panic(fmt.Sprintf("tool %q returned unsupported invocation: %T", toolName, inv))
		}

		if e.waiter == nil {
			panic("toolExecutor.executeInvocation: InteractiveInvocation encountered but no actionWaiter provided")
		}

		// Wait for the user to provide an answer/action (only returns context error handled by Resolve)
		action, _ := e.waiter.Wait(ctx, callID)

		// On success or user cancellation (ctx.Err() != nil), we ask the tool to resolve the final state.
		llmContent, finalDisplay, resErr := interInv.Resolve(ctx, action)
		if finalDisplay == nil {
			panic(fmt.Sprintf("tool %q Resolve returned nil finalDisplay (callID=%s)", toolName, callID))
		}
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
		}, finalDisplay, resErr
	}

	var streamWG sync.WaitGroup
	if si, ok := inv.(domain.StreamableInvocation); ok && events != nil {
		stream := si.Stream()
		if stream != nil {
			streamWG.Go(func() {
				buf := make([]byte, 1024*1024)
				for {
					n, err := stream.Read(buf)
					if n > 0 {
						events.SendUIUpdate(domain.ToolStreamEvent{
							CallID: callID,
							Chunk:  string(buf[:n]),
						})
					}
					if err != nil {
						break
					}
				}
			})
		}
	}

	llmContent, finalDisplay, err := execInv.Execute(ctx)
	if finalDisplay == nil {
		panic(fmt.Sprintf("tool %q Execute returned nil finalDisplay (callID=%s)", toolName, callID))
	}
	streamWG.Wait()
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
	}, finalDisplay, err
}

func (e *ToolExecutor) execute(ctx context.Context, tc *schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay, error) {
	plan, out := e.preflightCall(0, *tc, events)
	if plan == nil {
		return out.resp, out.display, nil
	}
	emitToolStart(events, plan.callID, plan.previewDisplay)
	return e.executeInvocation(ctx, tc.ID, tc.Function.Name, plan.inv, events)
}

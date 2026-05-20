package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/cloudwego/eino/compose"
)

const toolErrorPermissionDenied = "permission denied"

type previewer interface {
	Preview(input *compose.ToolInput) domain.ToolDisplay
}

type preflightValidator interface {
	PreflightValidate(input *compose.ToolInput) error
}

type permissionAsker interface {
	ShouldAsk(toolName string) bool
}

func newPreviewStartMiddleware(events eventSender, registry toolRegistry) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if events != nil {
					if display, ok := previewFromRegistry(registry, input); ok {
						events.SendUIUpdate(domain.ToolStartEvent{
							CallID:  input.CallID,
							Display: display,
						})
					}
				}
				return next(ctx, input)
			}
		},
	}
}

func newPermissionMiddleware(
	permissionAsker permissionAsker,
	waiter actionWaiter,
	events eventSender,
	registry toolRegistry,
) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if permissionAsker == nil || !permissionAsker.ShouldAsk(input.Name) {
					return next(ctx, input)
				}
				callID := input.CallID
				waitStart := time.Now()
				slog.Info("tool permission wait started", "tool", input.Name, "call_id", callID)
				if events != nil {
					events.SendUIUpdate(domain.ToolApprovalRequestEvent{CallID: callID})
				}
				if waiter == nil {
					slog.Error("tool permission waiter missing", "tool", input.Name, "call_id", callID)
					failedDisplay := resolveToolPreview(registry, input).WithError(domain.ToolErrorFailed)
					if events != nil {
						events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: failedDisplay})
					}
					if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
						sink(callID, failedDisplay)
					}
					return &compose.ToolOutput{Result: "Internal error: permission waiter unavailable"}, nil
				}
				act, waitErr := waiter.Wait(ctx, callID)
				if waitErr != nil {
					slog.Warn("tool permission wait failed", "tool", input.Name, "call_id", callID, "duration_ms", time.Since(waitStart).Milliseconds(), "error", waitErr)
					result := "Internal error: permission approval wait failed"
					toolErr := domain.ToolErrorFailed
					if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) || ctx.Err() != nil {
						toolErr = domain.ToolErrorCancelled
						result = domain.ToolErrorCancelled
					}
					display := resolveToolPreview(registry, input).WithError(toolErr)
					if events != nil {
						events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: display})
					}
					if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
						sink(callID, display)
					}
					return &compose.ToolOutput{Result: result}, nil
				}
				dec, ok := act.(domain.PermissionDecisionAction)
				if !ok || !dec.Approved {
					slog.Info("tool permission denied", "tool", input.Name, "call_id", callID, "duration_ms", time.Since(waitStart).Milliseconds())
					deniedDisplay := resolveToolPreview(registry, input).WithError(toolErrorPermissionDenied)
					if events != nil {
						events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: deniedDisplay})
					}
					if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
						sink(callID, deniedDisplay)
					}
					return &compose.ToolOutput{Result: "Tool execution was denied by the user."}, nil
				}
				slog.Info("tool permission approved", "tool", input.Name, "call_id", callID, "duration_ms", time.Since(waitStart).Milliseconds())
				return next(ctx, input)
			}
		},
	}
}

func newPreflightValidationMiddleware(events eventSender, registry toolRegistry) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				tl, ok := registry.Get(input.Name)
				if !ok {
					unknownDisplay := domain.NewStringDisplay("", "").WithError("Unknown tool request")
					if events != nil {
						events.SendUIUpdate(domain.ToolStartEvent{CallID: input.CallID, Display: unknownDisplay})
						events.SendUIUpdate(domain.ToolEndEvent{CallID: input.CallID, Display: unknownDisplay})
					}
					if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
						sink(input.CallID, unknownDisplay)
					}
					return &compose.ToolOutput{Result: fmt.Sprintf("Error: unknown tool %q", input.Name)}, nil
				}
				v, ok := tl.(preflightValidator)
				if !ok {
					return next(ctx, input)
				}
				if err := v.PreflightValidate(input); err != nil {
					callID := input.CallID
					deniedDisplay := domain.NewStringDisplay("", "").WithError(fmt.Sprintf("Bad %s request", strings.ToUpper(input.Name)))
					if events != nil {
						events.SendUIUpdate(domain.ToolStartEvent{CallID: callID, Display: deniedDisplay})
						events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: deniedDisplay})
					}
					if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok {
						sink(callID, deniedDisplay)
					}
					return &compose.ToolOutput{Result: fmt.Sprintf("Error: %v", err)}, nil
				}
				return next(ctx, input)
			}
		},
	}
}

// previewFromRegistry returns tool.Preview(input) only when the tool is registered, implements previewer,
// and Preview returns non-nil. Otherwise ok is false.
func previewFromRegistry(registry toolRegistry, input *compose.ToolInput) (domain.ToolDisplay, bool) {
	if registry == nil || input == nil {
		return nil, false
	}
	tl, ok := registry.Get(input.Name)
	if !ok {
		return nil, false
	}
	p, ok := tl.(previewer)
	if !ok {
		return nil, false
	}
	d := p.Preview(input)
	if d == nil {
		return nil, false
	}
	return d, true
}

// resolveToolPreview returns previewFromRegistry when available, else a generic Run "<name>" line
// (or empty when registry or input is nil).
func resolveToolPreview(registry toolRegistry, input *compose.ToolInput) domain.ToolDisplay {
	if d, ok := previewFromRegistry(registry, input); ok {
		return d
	}
	if registry == nil || input == nil {
		return domain.NewStringDisplay("", "")
	}
	return domain.NewStringDisplay(fmt.Sprintf("Run %q", input.Name), "")
}

func newExternalToolEventMiddleware(events eventSender, registry toolRegistry) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if registry == nil || events == nil {
					return next(ctx, input)
				}
				tl, ok := registry.Get(input.Name)
				if !ok {
					return next(ctx, input)
				}
				_, isPreview := tl.(previewer)
				if isPreview {
					return next(ctx, input)
				}

				// External/MCP tool: emit ToolStartEvent
				display := domain.NewStringDisplay(fmt.Sprintf("Run %q", input.Name), "")
				events.SendUIUpdate(domain.ToolStartEvent{
					CallID:  input.CallID,
					Display: display,
				})

				out, err := next(ctx, input)
				if err != nil {
					events.SendUIUpdate(domain.ToolEndEvent{
						CallID:  input.CallID,
						Display: display.WithError(err.Error()),
					})
					return nil, err
				}

				events.SendUIUpdate(domain.ToolEndEvent{
					CallID:  input.CallID,
					Display: domain.NewStringDisplay(fmt.Sprintf("Run %q", input.Name), ""),
				})
				return out, nil
			}
		},
	}
}


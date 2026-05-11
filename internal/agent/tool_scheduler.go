package agent

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type concurrentSafeTool interface {
	IsConcurrentSafe() bool
}

func (r *GraphRunner) graphRunTools(ctx context.Context, st *graphRunState) (*graphRunState, error) {
	last := lastAssistant(st.session.Messages)
	if last == nil || len(last.ToolCalls) == 0 {
		return st, nil
	}

	var safeBatch []schema.ToolCall
	for _, tc := range last.ToolCalls {
		if r.isToolCallConcurrentSafe(tc.Function.Name) {
			safeBatch = append(safeBatch, tc)
			continue
		}
		if err := r.appendToolOutputsForCalls(ctx, st, r.toolsNodeParallel, safeBatch); err != nil {
			return st, err
		}
		safeBatch = safeBatch[:0]
		if err := r.appendToolOutputsForCalls(ctx, st, r.toolsNodeSequential, []schema.ToolCall{tc}); err != nil {
			return st, err
		}
	}
	if err := r.appendToolOutputsForCalls(ctx, st, r.toolsNodeParallel, safeBatch); err != nil {
		return st, err
	}
	return st, nil
}

func (r *GraphRunner) appendToolOutputsForCalls(
	ctx context.Context,
	st *graphRunState,
	node *compose.ToolsNode,
	calls []schema.ToolCall,
) error {
	if len(calls) == 0 {
		return nil
	}
	out, err := r.invokeToolsForCalls(ctx, node, calls)
	if err != nil {
		return err
	}
	for _, msg := range out {
		if msg != nil {
			st.session.Messages = append(st.session.Messages, msg)
		}
	}
	return nil
}

func (r *GraphRunner) isToolCallConcurrentSafe(toolName string) bool {
	if toolName == "" {
		return false
	}
	if toolName == "ask_question" {
		return false
	}
	if r.permission != nil && r.permission.ShouldAsk(toolName) {
		return false
	}
	tl, ok := r.registry.Get(toolName)
	if !ok || tl == nil {
		return false
	}
	safe, ok := tl.(concurrentSafeTool)
	if !ok {
		return false
	}
	return safe.IsConcurrentSafe()
}

func (r *GraphRunner) invokeToolsForCalls(
	ctx context.Context,
	node *compose.ToolsNode,
	calls []schema.ToolCall,
) ([]*schema.Message, error) {
	if node == nil || len(calls) == 0 {
		return nil, nil
	}
	input := &schema.Message{
		Role:      schema.Assistant,
		ToolCalls: append([]schema.ToolCall(nil), calls...),
	}
	return node.Invoke(ctx, input)
}

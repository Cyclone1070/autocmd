package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// requestInterruptedByUserMessage is appended as a synthetic user message when the graph run context is cancelled.
const requestInterruptedByUserMessage = "[Request interrupted by user]"

// transcriptSummarizer compacts message history for context overflow. Implemented by *Summarizer.
type transcriptSummarizer interface {
	Summarize(ctx context.Context, msgs []*schema.Message) (*schema.Message, error)
}

type graphRunState struct {
	session     *domain.Session
	input       string
	iterations  int
	stopReason  error
}

type GraphRunner struct {
	llm          domain.LLM
	registry     toolRegistry
	waiter       actionWaiter
	events       eventSender
	notifier     taskNotifier
	summarizer   transcriptSummarizer
	maxIteration int
	permission   permissionAsker
	toolInfos    []*schema.ToolInfo
	toolsNodeParallel   *compose.ToolsNode
	toolsNodeSequential *compose.ToolsNode
	graph        compose.Runnable[*graphRunState, *graphRunState]
}

func graphMaxRunSteps(maxIterations int) int {
	if maxIterations <= 0 {
		maxIterations = 1
	}
	return maxIterations*4 + 20
}

func NewGraphRunner(
	llm domain.LLM,
	registry toolRegistry,
	waiter actionWaiter,
	maxIterations int,
	events eventSender,
	notifier taskNotifier,
	summarizer transcriptSummarizer,
	permission permissionAsker,
) (*GraphRunner, error) {
	if maxIterations <= 0 {
		maxIterations = 1
	}

	if llm == nil {
		return nil, fmt.Errorf("llm is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	tools := registry.Tools()
	if len(tools) == 0 {
		return nil, fmt.Errorf("tool registry has no tools")
	}

	toolInfos := make([]*schema.ToolInfo, 0, len(tools))
	for _, tl := range tools {
		info, err := tl.Info(context.Background())
		if err != nil {
			return nil, fmt.Errorf("tool info for %T: %w", tl, err)
		}
		toolInfos = append(toolInfos, info)
	}

	commonMiddlewares := []compose.ToolMiddleware{
		newPreflightValidationMiddleware(events, registry),
		newPreviewStartMiddleware(events, registry),
		newPermissionMiddleware(permission, waiter, events, registry),
	}
	toolsNodeParallel, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools: tools,
		ToolCallMiddlewares: commonMiddlewares,
	})
	if err != nil {
		return nil, fmt.Errorf("create parallel tools node: %w", err)
	}
	toolsNodeSequential, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools:               tools,
		ExecuteSequentially: true,
		ToolCallMiddlewares: commonMiddlewares,
	})
	if err != nil {
		return nil, fmt.Errorf("create sequential tools node: %w", err)
	}

	r := &GraphRunner{
		llm:          llm,
		registry:     registry,
		waiter:       waiter,
		events:       events,
		notifier:     notifier,
		summarizer:   summarizer,
		maxIteration: maxIterations,
		permission:   permission,
		toolInfos:    toolInfos,
		toolsNodeParallel:   toolsNodeParallel,
		toolsNodeSequential: toolsNodeSequential,
	}
	r.graph, err = r.buildGraph()
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *GraphRunner) buildGraph() (compose.Runnable[*graphRunState, *graphRunState], error) {
	g := compose.NewGraph[*graphRunState, *graphRunState]()

	if err := g.AddLambdaNode("preturn", compose.InvokableLambda(r.graphPreTurn)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("chat", compose.InvokableLambda(r.graphChatTurn)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("run_tools", compose.InvokableLambda(r.graphRunTools)); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finish", compose.InvokableLambda(func(_ context.Context, st *graphRunState) (*graphRunState, error) {
		return st, nil
	})); err != nil {
		return nil, err
	}

	if err := g.AddEdge(compose.START, "preturn"); err != nil {
		return nil, err
	}
	if err := g.AddBranch("preturn", compose.NewGraphBranch(
		func(_ context.Context, st *graphRunState) (string, error) {
			if st == nil || st.stopReason != nil {
				return "finish", nil
			}
			return "chat", nil
		},
		map[string]bool{"chat": true, "finish": true},
	)); err != nil {
		return nil, err
	}
	if err := g.AddBranch("chat", compose.NewGraphBranch(
		func(_ context.Context, st *graphRunState) (string, error) {
			if st == nil || st.stopReason != nil {
				return "finish", nil
			}
			last := lastAssistant(st.session.Messages)
			if last == nil || len(last.ToolCalls) == 0 {
				return "finish", nil
			}
			return "run_tools", nil
		},
		map[string]bool{"run_tools": true, "finish": true},
	)); err != nil {
		return nil, err
	}
	if err := g.AddEdge("run_tools", "preturn"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finish", compose.END); err != nil {
		return nil, err
	}

	return g.Compile(context.Background(), compose.WithMaxRunSteps(graphMaxRunSteps(r.maxIteration)))
}

func (r *GraphRunner) Run(ctx context.Context, session *domain.Session, input string) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	defer func() {
		if ctx.Err() != nil && len(session.Messages) > 0 {
			_ = appendMessageMerge(&session.Messages, &schema.Message{
				Role:    schema.User,
				Content: requestInterruptedByUserMessage,
			})
		}
	}()

	if err := appendMessageMerge(&session.Messages, &schema.Message{
		Role:    schema.User,
		Content: input,
	}); err != nil {
		return err
	}

	runCtx := ctx
	runCtx = runtimectx.WithActionWaiter(runCtx, r.waiter)
	runCtx = runtimectx.WithEventSender(runCtx, r.events)
	runCtx = runtimectx.WithToolDisplaySink(runCtx, func(callID string, display domain.ToolDisplay) {
		if display == nil || callID == "" {
			return
		}
		if session.ToolDisplays == nil {
			session.ToolDisplays = make(domain.ToolDisplays)
		}
		session.ToolDisplays[callID] = display
	})
	st := &graphRunState{session: session, input: input}
	out, err := r.graph.Invoke(runCtx, st)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: graph invoke: %w", classifyModelErr(err), err)
	}
	if out != nil && out.stopReason != nil {
		return out.stopReason
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func lastAssistant(msgs []*schema.Message) *schema.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i] != nil && msgs[i].Role == schema.Assistant {
			return msgs[i]
		}
	}
	return nil
}


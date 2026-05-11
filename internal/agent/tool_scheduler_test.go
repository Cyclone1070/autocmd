package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type concurrencyProbeTool struct {
	name           string
	delay          time.Duration
	concurrentSafe bool
	active         *atomic.Int32
	unsafeOverlap  *atomic.Bool
	orderMu        *sync.Mutex
	order          *[]string
}

func (t *concurrencyProbeTool) Name() string { return t.name }

func (t *concurrencyProbeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "probe tool",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"v": {Type: schema.String, Required: false},
		}),
	}, nil
}

func (t *concurrencyProbeTool) IsConcurrentSafe() bool { return t.concurrentSafe }

func (t *concurrencyProbeTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	t.orderMu.Lock()
	*t.order = append(*t.order, "start:"+t.name)
	t.orderMu.Unlock()

	cur := t.active.Add(1)
	if !t.concurrentSafe && cur > 1 {
		t.unsafeOverlap.Store(true)
	}
	time.Sleep(t.delay)
	t.active.Add(-1)

	t.orderMu.Lock()
	*t.order = append(*t.order, "end:"+t.name)
	t.orderMu.Unlock()
	return `{"ok":true}`, nil
}

func TestGraphRunner_RunTools_UnsafeCallIsBarrierAndDoesNotOverlap(t *testing.T) {
	active := &atomic.Int32{}
	unsafeOverlap := &atomic.Bool{}
	var order []string
	var orderMu sync.Mutex

	safe1 := &concurrencyProbeTool{
		name:           "safe1",
		delay:          40 * time.Millisecond,
		concurrentSafe: true,
		active:         active,
		unsafeOverlap:  unsafeOverlap,
		orderMu:        &orderMu,
		order:          &order,
	}
	unsafe := &concurrencyProbeTool{
		name:           "unsafe",
		delay:          40 * time.Millisecond,
		concurrentSafe: false,
		active:         active,
		unsafeOverlap:  unsafeOverlap,
		orderMu:        &orderMu,
		order:          &order,
	}
	safe2 := &concurrencyProbeTool{
		name:           "safe2",
		delay:          40 * time.Millisecond,
		concurrentSafe: true,
		active:         active,
		unsafeOverlap:  unsafeOverlap,
		orderMu:        &orderMu,
		order:          &order,
	}
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{
		"safe1":  safe1,
		"unsafe": unsafe,
		"safe2":  safe2,
	}}
	llm := &mockLLM{id: "mock", displayName: "Mock", contextWindow: 128_000}
	runner, err := NewGraphRunner(llm, reg, nil, 5, nil, nil, nil, nil)
	require.NoError(t, err)

	st := &graphRunState{
		session: &domain.Session{
			Messages: []*schema.Message{
				{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{
						{ID: "c1", Function: schema.FunctionCall{Name: "safe1", Arguments: `{}`}},
						{ID: "c2", Function: schema.FunctionCall{Name: "unsafe", Arguments: `{}`}},
						{ID: "c3", Function: schema.FunctionCall{Name: "safe2", Arguments: `{}`}},
					},
				},
			},
		},
	}
	_, err = runner.graphRunTools(context.Background(), st)
	require.NoError(t, err)
	require.False(t, unsafeOverlap.Load(), "unsafe tool must not overlap with other tool runs")

	require.GreaterOrEqual(t, len(order), 6)
	idxStartSafe1 := -1
	idxStartUnsafe := -1
	idxStartSafe2 := -1
	for i, ev := range order {
		switch ev {
		case "start:safe1":
			idxStartSafe1 = i
		case "start:unsafe":
			idxStartUnsafe = i
		case "start:safe2":
			idxStartSafe2 = i
		}
	}
	require.NotEqual(t, -1, idxStartSafe1)
	require.NotEqual(t, -1, idxStartUnsafe)
	require.NotEqual(t, -1, idxStartSafe2)
	require.Less(t, idxStartSafe1, idxStartUnsafe)
	require.Less(t, idxStartUnsafe, idxStartSafe2)
}

func TestGraphRunner_ToolConcurrency_AskQuestionNameNotSpecialCased(t *testing.T) {
	reg := &testToolRegistry{tools: map[string]tool.BaseTool{
		"ask_question": &concurrencyProbeTool{name: "ask_question", concurrentSafe: true},
	}}
	llm := &mockLLM{id: "mock", displayName: "Mock", contextWindow: 128_000}
	runner, err := NewGraphRunner(llm, reg, nil, 5, nil, nil, nil, nil)
	require.NoError(t, err)

	require.True(t, runner.isToolCallConcurrentSafe("ask_question"), "scheduler should rely on tool capability/policy, not hardcoded tool name")
}

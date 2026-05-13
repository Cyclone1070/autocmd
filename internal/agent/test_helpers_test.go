package agent

import (
	"context"
	"encoding/json"
	"io"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Shared literals for agent tests (goconst).
const (
	testToolNameGreet          = "greet"
	testMockLLMID              = "mock"
	testMockLLMDisplayName     = "Mock"
	testToolNameReadFile       = "read_file"
	testToolNameGrep           = "grep"
	testToolCallID1            = "call_1"
	testToolCallIDTC1          = "tc1"
	testToolOutputFromNext     = "from_next"
	testReadFileArgsJSON       = `{"file_path":"/tmp/x"}`
	testPermissionModeAsk      = "ask"
	testToolNameAskQuestion    = "ask_question"
	testConcurrencyProbeSafe1  = "safe1"
	testConcurrencyProbeUnsafe = "unsafe"
	testConcurrencyProbeSafe2  = "safe2"
)

type mockSummaryRunnable struct{}

func (mockSummaryRunnable) Invoke(ctx context.Context, input []*schema.Message, opts ...compose.Option) (*schema.Message, error) {
	_ = ctx
	_ = input
	_ = opts
	return &schema.Message{Role: schema.Assistant, Content: "summary"}, nil
}

func (mockSummaryRunnable) Stream(ctx context.Context, input []*schema.Message, opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	_ = ctx
	_ = input
	_ = opts
	return nil, io.EOF
}

func (mockSummaryRunnable) Collect(ctx context.Context, input *schema.StreamReader[[]*schema.Message], opts ...compose.Option) (*schema.Message, error) {
	_ = ctx
	_ = input
	_ = opts
	return nil, io.EOF
}

func (mockSummaryRunnable) Transform(ctx context.Context, input *schema.StreamReader[[]*schema.Message], opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	_ = ctx
	_ = input
	_ = opts
	return nil, io.EOF
}

type testToolRegistry struct {
	tools map[string]tool.BaseTool
}

func (m *testToolRegistry) Tools() []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(m.tools))
	for _, t := range m.tools {
		out = append(out, t)
	}
	return out
}

func (m *testToolRegistry) Get(name string) (tool.BaseTool, bool) {
	tl, ok := m.tools[name]
	return tl, ok
}

type greetTool struct {
	invokeCount int
}

func (g *greetTool) Name() string { return testToolNameGreet }

func (g *greetTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: testToolNameGreet,
		Desc: "greet with name",
		ParamsOneOf: schema.NewParamsOneOfByParams(
			map[string]*schema.ParameterInfo{
				"name": {
					Desc:     "user name",
					Required: true,
					Type:     schema.String,
				},
			}),
	}, nil
}

func (g *greetTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	g.invokeCount++
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", err
	}
	return `{"say":"hello ` + in.Name + `"}`, nil
}

func (g *greetTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	return domain.NewStringDisplay(testToolNameGreet, input.Arguments)
}

package tool

import (
	"context"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

// fakeTool is a tiny BaseTool for ordering tests.
type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string { return f.name }

func (f *fakeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: f.name, Desc: f.name}, nil
}

// Compile-time check it satisfies BaseTool through utils helper at least; we
// don't strictly need InvokableRun for these tests.
var _ einotool.BaseTool = (*fakeTool)(nil)

// Tools() feeds directly into the LLM request payload. Non-deterministic
// ordering (Go map iteration) varies across calls, busts the model's KV
// cache for tool prompts, and is observed to make thinking-capable Gemini
// models flake hard (long TTFB and Error 500). Lock the contract to a
// deterministic name-sorted ordering, matching the previous Definitions()
// behaviour.
func TestRegistry_Tools_DeterministicSortedByName(t *testing.T) {
	want := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}

	tools := []einotool.BaseTool{
		&fakeTool{name: "delta"},
		&fakeTool{name: "alpha"},
		&fakeTool{name: "foxtrot"},
		&fakeTool{name: "charlie"},
		&fakeTool{name: "bravo"},
		&fakeTool{name: "echo"},
	}
	r := NewRegistry(tools)

	for run := range 16 {
		got := r.Tools()
		gotNames := make([]string, len(got))
		for i, tl := range got {
			n, ok := tl.(interface{ Name() string })
			require.True(t, ok, "tool must expose Name()")
			gotNames[i] = n.Name()
		}
		require.Equal(t, want, gotNames, "Tools() must return tools sorted by name on every call (run %d)", run)
	}
}

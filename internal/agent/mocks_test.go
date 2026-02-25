package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Cyclone1070/iav/internal/domain"
)

// --- Mocks ---

type mockLLM struct {
	id            string
	displayName   string
	contextWindow int
	streams       []*mockStream
	streamErr     error
}

func (m *mockLLM) ID() string          { return m.id }
func (m *mockLLM) DisplayName() string { return m.displayName }
func (m *mockLLM) ContextWindow() int  { return m.contextWindow }

func (m *mockLLM) ComputeTokens(ctx context.Context, msgs []domain.Message) (int, error) {
	return 100, nil
}

func (m *mockLLM) Stream(ctx context.Context, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	if m.streamErr != nil && len(m.streams) == 0 {
		return nil, m.streamErr
	}
	if len(m.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

type mockStream struct {
	chunks []domain.StreamChunk
	err    error
	index  int
}

func (m *mockStream) Next() bool {
	if m.index < len(m.chunks) {
		m.index++
		return true
	}
	return false
}

func (m *mockStream) Chunk() domain.StreamChunk {
	return m.chunks[m.index-1]
}

func (m *mockStream) Err() error {
	return m.err
}

type mockTool struct {
	name        string
	description string
	prepare     func(ctx context.Context, params json.RawMessage) (domain.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) Declaration() domain.Declaration {
	return domain.Declaration{Name: mt.name, Description: mt.description}
}
func (mt *mockTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	if mt.prepare != nil {
		return mt.prepare(ctx, params)
	}
	return &mockInvocation{content: "ok"}, nil
}

type mockInvocation struct {
	content string
	err     error
	display domain.ToolDisplay
}

func (m *mockInvocation) Execute(ctx context.Context) (string, error) {
	return m.content, m.err
}
func (m *mockInvocation) Display() domain.ToolDisplay { return m.display }

type mockToolRegistry struct {
	tools map[string]domain.Tool
}

func newMockToolRegistry(tools []domain.Tool) *mockToolRegistry {
	m := &mockToolRegistry{tools: make(map[string]domain.Tool)}
	for _, t := range tools {
		if t != nil {
			m.tools[t.Name()] = t
		}
	}
	return m
}

func (m *mockToolRegistry) Declarations() []domain.Declaration {
	var decls []domain.Declaration
	for _, t := range m.tools {
		decls = append(decls, t.Declaration())
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

func (m *mockToolRegistry) Get(name string) (domain.Tool, bool) {
	t, ok := m.tools[name]
	return t, ok
}

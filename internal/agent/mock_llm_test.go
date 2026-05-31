package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type mockLLM struct {
	streamErr     error
	id            string
	displayName   string
	streams       []*mockStream
	contextWindow int
	LastMessages  []*schema.Message
}

func (m *mockLLM) ID() string          { return m.id }
func (m *mockLLM) DisplayName() string { return m.displayName }
func (m *mockLLM) ContextWindow() int  { return m.contextWindow }
func (m *mockLLM) Model() model.ToolCallingChatModel {
	return &mockEinoModelBridge{llm: m}
}

type mockEinoModelBridge struct {
	llm *mockLLM
}

func (b *mockEinoModelBridge) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	sr, err := b.Stream(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	defer sr.Close()
	var chunks []*schema.Message
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return schema.ConcatMessages(chunks)
}

func (b *mockEinoModelBridge) Stream(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	b.llm.LastMessages = msgs
	if b.llm.streamErr != nil && len(b.llm.streams) == 0 {
		return nil, b.llm.streamErr
	}
	if len(b.llm.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := b.llm.streams[0]
	b.llm.streams = b.llm.streams[1:]
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		for _, chunk := range s.chunks {
			msg := &schema.Message{
				Role:             schema.Assistant,
				Content:          chunk.text,
				ReasoningContent: chunk.reasoningContent,
				ToolCalls:        chunk.toolCalls,
			}
			sw.Send(msg, nil)
		}
		if s.err != nil {
			sw.Send(nil, s.err)
		}
	}()
	return sr, nil
}

func (b *mockEinoModelBridge) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return b, nil
}

type mockChunk struct {
	text             string
	reasoningContent string
	toolCalls        []schema.ToolCall
}

type mockStream struct {
	err    error
	chunks []mockChunk
}

var _ domain.LLM = (*mockLLM)(nil)

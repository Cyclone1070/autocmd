package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/Cyclone1070/iav/internal/domain"
	flowagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
)

func (r *GraphRunner) graphChatTurn(ctx context.Context, st *graphRunState) (*graphRunState, error) {
	st.iterations++
	slog.Info("graph chat turn start", "iteration", st.iterations, "messages", len(st.session.Messages))
	modelWithTools, err := flowagent.ChatModelWithTools(nil, r.llm.Model(), r.toolInfos)
	if err != nil {
		slog.Error("graph chat bind tools failed", "error", err, "error_text", err.Error())
		return st, fmt.Errorf("%w: bind tools: %w", classifyModelErr(err), err)
	}
	stream, err := modelWithTools.Stream(ctx, st.session.Messages)
	if err != nil {
		slog.Error("graph chat stream start failed", "error", err, "error_text", err.Error())
		return st, fmt.Errorf("%w: LLM.Stream: %w", classifyModelErr(err), err)
	}
	defer stream.Close()

	var chunks []*schema.Message
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			slog.Error("graph chat stream recv failed", "error", err, "error_text", err.Error(), "chunks_received", len(chunks))
			if appendErr := graphAppendConcatenatedAssistantMessage(st.session, chunks); appendErr != nil {
				slog.Error("graph chat append partial assistant failed", "error", appendErr, "error_text", appendErr.Error())
				return st, appendErr
			}
			return st, fmt.Errorf("%w: reader.Recv: %w", classifyModelErr(err), err)
		}
		chunks = append(chunks, chunk)
		if r.events != nil {
			if chunk.Content != "" {
				r.events.SendUIUpdate(domain.TextEvent{Text: chunk.Content, IsThought: false})
			}
			if chunk.ReasoningContent != "" {
				r.events.SendUIUpdate(domain.TextEvent{Text: chunk.ReasoningContent, IsThought: true})
			}
		}
	}
	if err := graphAppendConcatenatedAssistantMessage(st.session, chunks); err != nil {
		slog.Error("graph chat append assistant failed", "error", err, "error_text", err.Error())
		return st, err
	}
	last := lastAssistant(st.session.Messages)
	if last != nil {
		slog.Info(
			"graph chat turn complete",
			"assistant_tool_calls", len(last.ToolCalls),
			"assistant_content_len", len(last.Content),
		)
	}
	return st, nil
}

func graphAppendConcatenatedAssistantMessage(session *domain.Session, chunks []*schema.Message) error {
	if len(chunks) == 0 {
		return nil
	}
	normalizeToolCallIndices(chunks)
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return fmt.Errorf("ConcatMessages: %w", err)
	}
	return appendMessageMerge(&session.Messages, msg)
}

// normalizeToolCallIndices ensures every unique tool call ID has a stable, unique index.
// Some providers emit colliding or missing indices for parallel tools.
func normalizeToolCallIndices(chunks []*schema.Message) {
	idToFixedIdx := make(map[string]int)
	origToFixedIdx := make(map[int]int)
	nextAvailableIdx := 0

	for _, chunk := range chunks {
		for i := range chunk.ToolCalls {
			tc := &chunk.ToolCalls[i]

			origIdx := 0
			if tc.Index != nil {
				origIdx = *tc.Index
			}

			if tc.ID != "" {
				fixedIdx, ok := idToFixedIdx[tc.ID]
				if !ok {
					fixedIdx = nextAvailableIdx
					idToFixedIdx[tc.ID] = fixedIdx
					nextAvailableIdx++
				}
				tc.Index = &fixedIdx
				origToFixedIdx[origIdx] = fixedIdx
			} else if tc.Index != nil {
				if fixedIdx, ok := origToFixedIdx[origIdx]; ok {
					tc.Index = &fixedIdx
				}
			}
		}
	}
}

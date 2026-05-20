package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

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
		return st, fmt.Errorf("%w: bind tools: %w", ErrModel, err)
	}
	stream, err := modelWithTools.Stream(ctx, st.session.Messages)
	if err != nil {
		slog.Error("graph chat stream start failed", "error", err, "error_text", err.Error())
		return st, fmt.Errorf("%w: LLM.Stream: %w", ErrModel, err)
	}
	defer stream.Close()

	var chunks []*schema.Message
	var reasoningStart time.Time
	var reasoningStarted bool
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			slog.Error("graph chat stream recv failed", "error", err, "error_text", err.Error(), "chunks_received", len(chunks))
			if appendErr := appendConcatenatedAssistantTurn(st.session, chunks, reasoningStarted, reasoningStart); appendErr != nil {
				slog.Error("graph chat append partial assistant failed", "error", appendErr, "error_text", appendErr.Error())
				return st, appendErr
			}
			return st, fmt.Errorf("%w: reader.Recv: %w", ErrModel, err)
		}
		if chunk.ReasoningContent != "" && !reasoningStarted {
			reasoningStart = time.Now()
			reasoningStarted = true
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
	if err := appendConcatenatedAssistantTurn(st.session, chunks, reasoningStarted, reasoningStart); err != nil {
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

// buildConcatenatedAssistantMessage returns nil when chunks is empty.
func buildConcatenatedAssistantMessage(chunks []*schema.Message) (*schema.Message, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	normalizeToolCallIndices(chunks)
	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, fmt.Errorf("ConcatMessages: %w", err)
	}
	return msg, nil
}

func applyThoughtDurationExtra(msg *schema.Message, reasoningStarted bool, reasoningStart, end time.Time) {
	if msg == nil || !reasoningStarted {
		return
	}
	if strings.TrimSpace(msg.ReasoningContent) == "" {
		return
	}
	d := max(time.Duration(0), end.Sub(reasoningStart))
	if msg.Extra == nil {
		msg.Extra = make(map[string]any)
	}
	msg.Extra[domain.ThoughtDurationMsExtraKey] = d.Milliseconds()
}

func appendConcatenatedAssistantTurn(session *domain.Session, chunks []*schema.Message, reasoningStarted bool, reasoningStart time.Time) error {
	msg, err := buildConcatenatedAssistantMessage(chunks)
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}
	applyThoughtDurationExtra(msg, reasoningStarted, reasoningStart, time.Now())
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

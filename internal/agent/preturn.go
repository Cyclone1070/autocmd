package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

func (r *GraphRunner) graphPreTurn(ctx context.Context, st *graphRunState) (*graphRunState, error) {
	if st == nil || st.session == nil {
		return st, fmt.Errorf("session is required")
	}
	if st.iterations >= r.maxIteration {
		st.session.Messages = append(st.session.Messages, &schema.Message{
			Role:    schema.User,
			Content: "[Max iterations reached]",
		})
		st.stopReason = fmt.Errorf("max iterations (%d) reached", r.maxIteration)
		return st, nil
	}

	msgs := st.session.Messages
	cw := r.llm.ContextWindow()
	if cw <= 0 {
		cw = 8192
	}
	threshold := int(float64(cw) * graphContextSummaryThreshold)
	totalTokens := st.session.TotalTokens()
	if r.summarizer != nil && len(msgs) > 1 && totalTokens > threshold {
		lastRole := msgs[len(msgs)-1].Role
		slog.Info(
			"graph preturn compaction triggered",
			"messages", len(msgs),
			"total_tokens", totalTokens,
			"threshold_tokens", threshold,
			"last_role", lastRole,
		)
		last := msgs[len(msgs)-1]
		history := msgs[:len(msgs)-1]
		summary, err := r.summarizer.Summarize(ctx, history)
		if err == nil {
			last.Content = fmt.Sprintf(
				"[Conversation compacted automatically]\n\n%s\n\n=== CURRENT REQUEST ===\n\n%s",
				summary.Content,
				last.Content,
			)
			msgs = []*schema.Message{last}
			slog.Info(
				"graph preturn compaction applied",
				"messages_after", len(msgs),
				"last_role_after", last.Role,
			)
		} else {
			slog.Warn("graph preturn compaction summarize failed", "error", err, "error_text", err.Error())
		}
	}
	if r.notifier != nil {
		for _, xml := range r.notifier.Drain() {
			msgs = append(msgs, &schema.Message{
				Role:    schema.User,
				Content: xml,
				Extra:   map[string]any{domain.NotificationMessageExtraKey: true},
			})
		}
	}
	st.session.Messages = msgs
	return st, nil
}

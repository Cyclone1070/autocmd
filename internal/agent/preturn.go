package agent

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const graphContextSummaryThreshold = 0.8

func (r *GraphRunner) graphPreTurn(ctx context.Context, st *graphRunState) (*graphRunState, error) {
	if st == nil || st.session == nil {
		return st, fmt.Errorf("session is required")
	}
	if st.iterations >= r.maxIteration {
		if err := appendMessageMerge(&st.session.Messages, &schema.Message{
			Role:    schema.User,
			Content: "[Max iterations reached]",
		}); err != nil {
			return st, err
		}
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
		if r.events != nil {
			r.events.SendUIUpdate(domain.SummaryCompactionStartEvent{})
		}
		lastRole := msgs[len(msgs)-1].Role
		slog.Info(
			"graph preturn compaction triggered",
			"messages", len(msgs),
			"total_tokens", totalTokens,
			"threshold_tokens", threshold,
			"last_role", lastRole,
		)
		last := msgs[len(msgs)-1]
		var replacement []*schema.Message
		var summarizeErr error
		var summary *schema.Message

		if last.Role == schema.User {
			history := msgs[:len(msgs)-1]
			summary, summarizeErr = r.summarizer.Summarize(ctx, history)
			if summarizeErr == nil && summary != nil {
				prefix := "[Conversation compacted automatically]\n\n" + summary.Content + "\n\n=== CURRENT REQUEST ==="
				part := &schema.Message{Role: schema.User, Content: prefix}
				var out []*schema.Message
				if err := appendMessageMerge(&out, part); err != nil {
					return st, err
				}
				if err := appendMessageMerge(&out, last); err != nil {
					return st, err
				}
				replacement = out
			}
		} else {
			summary, summarizeErr = r.summarizer.Summarize(ctx, msgs)
			if summarizeErr == nil && summary != nil {
				comp := "[Conversation compacted automatically]\n\n" + summary.Content
				replacement = []*schema.Message{{Role: schema.User, Content: comp}}
			}
		}

		if summarizeErr != nil {
			if r.events != nil {
				r.events.SendUIUpdate(domain.SummaryCompactionEndEvent{Error: summarizeErr.Error()})
			}
			slog.Warn("graph preturn compaction summarize failed", "error", summarizeErr, "error_text", summarizeErr.Error())
			return st, fmt.Errorf("graph preturn compaction: summarize: %w", summarizeErr)
		}
		if len(replacement) == 0 {
			errNil := fmt.Errorf("summarize returned nil message")
			if r.events != nil {
				r.events.SendUIUpdate(domain.SummaryCompactionEndEvent{Error: errNil.Error()})
			}
			slog.Warn("graph preturn compaction summarize failed", "error", errNil, "error_text", errNil.Error())
			return st, fmt.Errorf("graph preturn compaction: %w", errNil)
		}
		msgs = replacement
		if r.events != nil {
			r.events.SendUIUpdate(domain.SummaryCompactionEndEvent{})
		}
		slog.Info(
			"graph preturn compaction applied",
			"messages_after", len(msgs),
			"last_role_after", msgs[len(msgs)-1].Role,
		)
	}
	if r.notifier != nil {
		for _, res := range r.notifier.Drain() {
			type notificationWrapper struct {
				XMLName string `xml:"system-notification"`
				Type    string `xml:"type,attr"`
				domain.TaskResult
				Note string `xml:"note"`
			}
			wrapper := notificationWrapper{
				Type:       "task_completion",
				TaskResult: res,
				Note:       "Message auto generated. User doesn't see this message - write your response accordingly",
			}
			xmlBytes, err := xml.MarshalIndent(wrapper, "", "  ")
			if err != nil {
				return st, fmt.Errorf("marshal notification: %w", err)
			}
			xml := string(xmlBytes)

			if err := appendMessageMerge(&msgs, &schema.Message{
				Role:    schema.User,
				Content: xml,
				Extra:   map[string]any{domain.NotificationMessageExtraKey: true},
			}); err != nil {
				return st, err
			}

			// Emit SystemNotificationEvent with the content!
			if r.events != nil {
				r.events.SendUIUpdate(domain.SystemNotificationEvent{Content: xml})
			}
		}
	}
	st.session.Messages = msgs
	return st, nil
}

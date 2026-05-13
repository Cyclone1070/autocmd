package history

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/prompt"
	"github.com/cloudwego/eino/schema"
)

// formatThoughtDurationLabel matches the live thinking footer
// (rounded to the nearest second, Go duration string).
func formatThoughtDurationLabel(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(time.Second).String()
}

func thoughtDurationMSFromExtra(msg *schema.Message) (int64, bool) {
	if msg == nil || msg.Extra == nil {
		return 0, false
	}
	v, ok := msg.Extra[domain.ThoughtDurationMsExtraKey]
	if !ok {
		return 0, false
	}
	return extraJSONNumberAsInt64(v)
}

func extraJSONNumberAsInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case float64:
		return int64(x), true
	case float32:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func thoughtHistoryLine(b *Builder, am *schema.Message) string {
	if b == nil || am == nil {
		return ""
	}
	if strings.TrimSpace(am.ReasoningContent) == "" {
		return ""
	}
	ms, ok := thoughtDurationMSFromExtra(am)
	if !ok {
		return ""
	}
	tr := prompt.NewThinkingRenderer(b.Theme, b.Width, nil)
	return tr.RenderCompletedThought(formatThoughtDurationLabel(ms), 0, nil)
}

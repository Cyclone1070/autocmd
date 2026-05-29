package history

import (
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestFormatThoughtDurationLabel_KnownCases(t *testing.T) {
	require.Equal(t, "0s", formatThoughtDurationLabel(0))
	require.Equal(t, "1s", formatThoughtDurationLabel(500))
	require.Equal(t, "2s", formatThoughtDurationLabel(1500))
	require.Equal(t, "2s", formatThoughtDurationLabel(2000))
	require.Equal(
		t,
		(1234 * time.Millisecond).Round(time.Second).String(),
		formatThoughtDurationLabel(1234),
	)
}

func TestThoughtDurationMSFromExtra(t *testing.T) {
	t.Run("nil message", func(t *testing.T) {
		_, ok := thoughtDurationMSFromExtra(nil)
		require.False(t, ok)
	})
	t.Run("nil extra", func(t *testing.T) {
		_, ok := thoughtDurationMSFromExtra(&schema.Message{Role: schema.Assistant})
		require.False(t, ok)
	})
	t.Run("missing key", func(t *testing.T) {
		_, ok := thoughtDurationMSFromExtra(&schema.Message{Extra: map[string]any{}})
		require.False(t, ok)
	})
	t.Run("int64", func(t *testing.T) {
		ms, ok := thoughtDurationMSFromExtra(&schema.Message{
			Extra: map[string]any{domain.ThoughtDurationMsExtraKey: int64(42)},
		})
		require.True(t, ok)
		require.Equal(t, int64(42), ms)
	})
	t.Run("float64 JSON", func(t *testing.T) {
		ms, ok := thoughtDurationMSFromExtra(&schema.Message{
			Extra: map[string]any{domain.ThoughtDurationMsExtraKey: float64(99)},
		})
		require.True(t, ok)
		require.Equal(t, int64(99), ms)
	})
	t.Run("int", func(t *testing.T) {
		ms, ok := thoughtDurationMSFromExtra(&schema.Message{
			Extra: map[string]any{domain.ThoughtDurationMsExtraKey: 7},
		})
		require.True(t, ok)
		require.Equal(t, int64(7), ms)
	})
}

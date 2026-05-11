package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/stretchr/testify/require"
)

func TestFormatSpyEventLine(t *testing.T) {
	line := formatSpyEventLine(1500*time.Millisecond, domain.TextEvent{Text: "hello", IsThought: false})
	require.True(t, strings.Contains(line, "t=1.500s"))
	require.True(t, strings.Contains(line, "event=domain.TextEvent"))
	require.True(t, strings.Contains(line, "payload={Text:hello IsThought:false}"))
}

func TestHandleAutoActions_PermissionRequest_AutoApproves(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	ok := handleAutoActions(context.Background(), bus, domain.ToolApprovalRequestEvent{CallID: "c1"}, true)
	require.True(t, ok)

	select {
	case act := <-bus.WorkflowActions():
		dec, ok := act.(domain.PermissionDecisionAction)
		require.True(t, ok)
		require.Equal(t, "c1", dec.CallID)
		require.True(t, dec.Approved)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected auto approval action")
	}
}


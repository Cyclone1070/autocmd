package compose

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

func TestCompose_NewRenderer_EndToEndStream(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = 80
	r, err := NewRenderer(&buf, bytes.NewReader(nil), cfg)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = r.Wait()
		close(done)
	}()
	r.Send(domain.TextEvent{Text: "Hello\n\n"})
	r.Send(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Tool A")})
	r.Send(domain.ToolEndEvent{CallID: "t1"})
	r.Send(domain.DoneEvent{})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return within 5s")
	}
	output := buf.String()
	if !strings.Contains(output, "Hello") {
		t.Errorf("expected 'Hello' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Tool A") {
		t.Errorf("expected 'Tool A' in output, got:\n%s", output)
	}
}

func TestCompose_ToolOrdering(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = 80
	r, err := NewRenderer(&buf, bytes.NewReader(nil), cfg)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = r.Wait()
		close(done)
	}()
	r.Send(domain.ToolStartEvent{
		CallID:   "call_A",
		ToolName: "slow-tool",
		Display:  domain.StringDisplay("Tool A Running..."),
	})
	r.Send(domain.ToolStartEvent{
		CallID:   "call_B",
		ToolName: "fast-tool",
		Display:  domain.StringDisplay("Tool B Running..."),
	})
	r.Send(domain.ToolEndEvent{CallID: "call_B"})
	r.Send(domain.ToolEndEvent{CallID: "call_A"})
	r.Send(domain.DoneEvent{})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return within 5s")
	}
	output := buf.String()
	idxA := strings.Index(output, "Tool A Running")
	idxB := strings.Index(output, "Tool B Running")
	if idxA == -1 || idxB == -1 {
		t.Fatalf("expected both tools in output, got:\n%s", output)
	}
	if idxA > idxB {
		t.Errorf("Tool A should appear before Tool B, but A at %d, B at %d", idxA, idxB)
	}
}

func TestCompose_DoneFlushesAllContent(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = 80
	r, err := NewRenderer(&buf, bytes.NewReader(nil), cfg)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = r.Wait()
		close(done)
	}()
	r.Send(domain.ToolStartEvent{
		CallID:   "t1",
		ToolName: "test",
		Display:  domain.StringDisplay("Tool Output"),
	})
	r.Send(domain.TextEvent{Text: "Some new "})
	r.Send(domain.TextEvent{Text: "text without "})
	r.Send(domain.TextEvent{Text: "flush..."})
	r.Send(domain.ToolEndEvent{CallID: "t1"})
	r.Send(domain.DoneEvent{})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return within 5s")
	}
	output := buf.String()
	if !strings.Contains(output, "Tool Output") {
		t.Error("Tool output missing")
	}
	if !strings.Contains(output, "Some new text") {
		t.Error("Pending text missing")
	}
}

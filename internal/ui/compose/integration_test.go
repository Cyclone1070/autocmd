package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
)

func TestNewPath_OrderingParity(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)

	h.ApplyEvent(domain.ToolStartEvent{CallID: "a", ToolName: "x", Display: domain.StringDisplay("TOOL_A")}, "toolA")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "a"}, "endA")
	h.ApplyEvent(domain.DoneEvent{}, "done")

	transcript := h.FullTranscript()

	if !strings.Contains(transcript, "TOOL_A") {
		t.Errorf("missing TOOL_A in transcript:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Context:") {
		t.Errorf("missing Context: in transcript")
	}
}

func TestArchitecture_NoUnsunkTerminalWrites(t *testing.T) {
	uiDir := ".."
	allowedFile := "tea/frame.go"
	var violations []string
	err := filepath.Walk(uiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(uiDir, path)
		if rel == filepath.Join("tea", "frame.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(content)
		if strings.Contains(s, "bubbletea.Println(") || strings.Contains(s, "bubbletea.Printf(") {
			violations = append(violations, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("bubbletea.Println/Printf must only appear in %s; found in: %v", allowedFile, violations)
	}
}

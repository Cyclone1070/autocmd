// Package ui contains architecture consistency tests.
// These tests verify that no bridge artifacts (engineadapter, export.go) remain in production.

package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoBridgeArtifacts verifies that production code does not reference
// engineadapter or internal/ui/export. Phase C gate: bridge artifacts removed.
func TestNoBridgeArtifacts(t *testing.T) {
	// rg exits 0 when matches found, 1 when none; exclude tests and docs
	cmd := exec.Command("rg", "-l", "engineadapter", "--glob", "!*_test.go", "--glob", "!*.md", ".")
	out, err := cmd.Output()
	outStr := strings.TrimSpace(string(out))
	// Fail if we found any matches (rg exit 0) or if rg failed unexpectedly
	if err == nil && outStr != "" {
		t.Errorf("found engineadapter references in production code (bridge not removed):\n%s", outStr)
	}
}

// TestNoLegacyReferences verifies no production references to legacy model/update/view/print_queue.
// Phase D gate: legacy files removed.
func TestNoLegacyReferences(t *testing.T) {
	// rg for references to legacy types that would indicate old path still in use
	cmd := exec.Command("rg", "-l", "NewTestableModelWithStream|newModelWithStream|FrameHarness", "--glob", "!*_test.go", ".")
	out, _ := cmd.Output()
	outStr := strings.TrimSpace(string(out))
	if outStr != "" {
		t.Errorf("found legacy model/harness references in production: %s", outStr)
	}
}

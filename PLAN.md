# Implementation Plan: Fix Frozen Spinner in Tool Displays

## Problem Statement

**Issue**: Spinners in tool displays (Read, Edit, Shell, etc.) are not animating when tools are in "Running" state. The spinner appears frozen even though the status bar spinner animates correctly.

**Root Cause**: The `ViewTool` function captures a stale spinner closure at initialization time. When the spinner animates via tick messages, it updates `a.Spinner` in `TeaModelAdapter`, but the `ViewTool` closure references the original local variable `s` from `NewTeaModelAdapter`, which is never updated. This is a closure capture bug.

**Solution**: Refactor `ViewTool` from a bare function with captured closure to a proper `ToolRenderer` interface/struct with dependency injection. Pass the current spinner explicitly at render time instead of capturing it at creation time.

---

## Implementation Approach: Test-Driven Development (TDD)

This plan follows strict TDD methodology:

1. **RED**: Write a failing test that exposes the bug
2. **GREEN**: Implement the minimal fix to make the test pass
3. **REFACTOR**: Update existing tests and clean up code

---

## Phase 1: RED - Write Failing Test

### Step 1.1: Create Integration Test

**File**: `internal/ui/tea/model_adapter_test.go`

**Test**: `TestSpinnerAnimatesInRunningTool`

**Purpose**: Verify that when a tool is in Running state and spinner ticks occur, the rendered view changes (spinner animates).

**Implementation**:

```go
func TestSpinnerAnimatesInRunningTool(t *testing.T) {
	// Setup: Create a minimal tool in Running state
	geom := engine.Geometry{Width: 80, TermHeight: 24, SpaceBelow: 20}
	state := engine.NewInitialState(geom)
	
	// Add a tool in Running state
	state.Tools = append(state.Tools, &engine.ToolState{
		CallID:  "test-tool-1",
		Display: domain.StringDisplay("Reading file..."),
		Status:  engine.StatusRunning,
		Err:     "",
	})
	
	// Create deps factory (current implementation)
	factory := func(s *spinner.Model) engine.Deps {
		// This is the current buggy implementation for the test
		return engine.Deps{
			Markdown: &mockMarkdownStream{},
			Theme:    &mockThemeAdapter{},
			Layout:   &mockLayoutAdapter{},
			ViewTool: func(ts *engine.ToolState) string {
				// Simplified mock that uses captured spinner
				return fmt.Sprintf("[%s] %s", s.View(), ts.Display)
			},
			Spinner: nil,
		}
	}
	
	sink := &RecordingSink{Events: []FrameEvent{}}
	adapter := NewTeaModelAdapter(state, factory, sink)
	
	// Capture initial view
	view1 := adapter.View()
	
	// Send 3 spinner ticks
	for i := 0; i < 3; i++ {
		tickMsg := spinner.TickMsg{}
		adapter.Update(tickMsg)
	}
	
	// Capture view after ticks
	view2 := adapter.View()
	
	// ASSERTION: Views should be different (spinner animated)
	if view1 == view2 {
		t.Errorf("Spinner did not animate. View remained identical after 3 ticks.\nView: %s", view1)
	}
	
	// Additional assertion: Both views should contain tool display
	if !strings.Contains(view1, "Reading file") || !strings.Contains(view2, "Reading file") {
		t.Errorf("Tool display not rendered correctly")
	}
}
```

**Mock Helpers** (add to `model_adapter_test.go`):

```go
type mockMarkdownStream struct{}
func (m *mockMarkdownStream) Append(chunk string) ([]string, error) { return nil, nil }
func (m *mockMarkdownStream) Pending() string { return "" }
func (m *mockMarkdownStream) RenderRemaining() (string, error) { return "", nil }

type mockThemeAdapter struct{}
func (m *mockThemeAdapter) Success(s string) string { return s }
func (m *mockThemeAdapter) Error(s string) string { return s }
func (m *mockThemeAdapter) Muted(s string) string { return s }
func (m *mockThemeAdapter) Primary(s string) string { return s }
func (m *mockThemeAdapter) Box(content string, width int, status engine.ToolStatus) string { return content }
func (m *mockThemeAdapter) Separator(width int, status engine.ToolStatus) string { return "---" }
func (m *mockThemeAdapter) SpinnerStyle() string { return "" }

type mockLayoutAdapter struct{}
func (m *mockLayoutAdapter) TruncateWithIndicator(content string, termHeight int) string { return content }
```

### Step 1.2: Run Test and Confirm Failure

**Command**:
```bash
cd internal/ui/tea
go test -v -run TestSpinnerAnimatesInRunningTool
```

**Expected Result**: 
```
--- FAIL: TestSpinnerAnimatesInRunningTool (0.00s)
    model_adapter_test.go:XX: Spinner did not animate. View remained identical after 3 ticks.
```

**Success Criteria**: Test MUST fail with the message indicating spinner did not animate.

---

## Phase 2: GREEN - Implement Fix

### Step 2.1: Define ToolRenderer Interface

**File**: `internal/ui/engine/contracts.go`

**Action**: Add new interface after `SpinnerViewProvider`

```go
// ToolRenderer renders tool displays for the UI.
// Consumer-defined; implemented by compose package.
type ToolRenderer interface {
	Render(t *ToolState, spinner SpinnerViewProvider) string
}
```

**Rationale**: This interface follows the existing pattern where consumer (engine) defines the interface and provider (compose) implements it.

### Step 2.2: Update Deps Struct

**File**: `internal/ui/engine/types.go`

**Action**: Change line 53 from:
```go
ViewTool  func(*ToolState) string // Renders a tool for display
```

To:
```go
ToolRenderer ToolRenderer // Renders a tool for display
```

**Rationale**: Replace bare function with interface-based dependency injection.

### Step 2.3: Update Call Sites in Engine

**File**: `internal/ui/engine/transition.go`

**Action 1**: Update line 130:
```go
// OLD:
output := deps.ViewTool(ts)

// NEW:
output := deps.ToolRenderer.Render(ts, deps.Spinner)
```

**Action 2**: Update line 222:
```go
// OLD:
parts = append(parts, deps.ViewTool(t))

// NEW:
parts = append(parts, deps.ToolRenderer.Render(t, deps.Spinner))
```

**Rationale**: Pass the current spinner explicitly instead of relying on captured closure.

### Step 2.4: Implement ToolRenderer in Compose

**File**: `internal/ui/compose/deps.go`

**Action 1**: Add struct implementation (add after `layoutAdapter`):

```go
// toolRenderer implements engine.ToolRenderer.
type toolRenderer struct {
	theme       *theme.Theme
	shellHeight int
	width       int
}

// newToolRenderer creates a new tool renderer with injected dependencies.
func newToolRenderer(cfg *config.Config, width int) *toolRenderer {
	return &toolRenderer{
		theme:       theme.NewTheme(cfg.UI),
		shellHeight: cfg.UI.ShellOutputHeight,
		width:       width,
	}
}

// Render implements engine.ToolRenderer.Render.
func (r *toolRenderer) Render(t *engine.ToolState, spinner engine.SpinnerViewProvider) string {
	status := toToolStatus(t.Status)
	
	// Get spinner view at render time (not at creation time!)
	var prefix string
	switch status {
	case theme.StatusRunning:
		if spinner != nil {
			prefix = spinner.SpinnerView()
		}
	case theme.StatusSuccess:
		prefix = r.theme.Success("✓")
	case theme.StatusError:
		prefix = r.theme.Error("✗")
	}
	
	contentWidth := r.width - 2
	var content string
	switch d := t.Display.(type) {
	case domain.StringDisplay:
		content = tool.RenderString(r.theme, d, status, t.Err, prefix)
	case domain.DiffDisplay:
		content = tool.RenderDiff(contentWidth, r.theme, d, status, t.Err, prefix)
	case domain.ShellDisplay:
		content = tool.RenderShell(contentWidth, r.shellHeight, r.theme, d, t.ShellOutput, status, t.Err, prefix)
	default:
		content = tool.Pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}
	
	return r.theme.Box(content, contentWidth, status)
}
```

**Action 2**: Update `NewEngineDeps` signature (line 19):

```go
// OLD:
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int, getSpinnerView func() string) engine.Deps {

// NEW:
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int) engine.Deps {
```

**Action 3**: Update `NewEngineDeps` body:

```go
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int) engine.Deps {
	return engine.Deps{
		Markdown:     sm,
		Theme:        &themeAdapter{t: theme.NewTheme(cfg.UI)},
		Layout:       layoutAdapter{},
		ToolRenderer: newToolRenderer(cfg, width), // NEW: Use struct instead of closure
		Spinner:      nil, // Set at runtime
	}
}
```

**Action 4**: Delete the old `viewTool` function (lines 69-93) - it's now replaced by `toolRenderer.Render`.

### Step 2.5: Update Compose Factory

**File**: `internal/ui/compose/compose.go`

**Action**: Update factory function (lines 45-49):

```go
// OLD:
factory := func(s *spinner.Model) engine.Deps {
	deps := NewEngineDeps(cfg, sm, width, func() string { return s.View() })
	deps.Spinner = nil
	return deps
}

// NEW:
factory := func(s *spinner.Model) engine.Deps {
	deps := NewEngineDeps(cfg, sm, width) // Removed spinner closure!
	deps.Spinner = nil // Still set at runtime in View()
	return deps
}
```

**Rationale**: No need to pass spinner closure anymore. The ToolRenderer will receive spinner at render time.

### Step 2.6: Run Test and Confirm Pass

**Command**:
```bash
cd internal/ui/tea
go test -v -run TestSpinnerAnimatesInRunningTool
```

**Expected Result**: 
```
--- PASS: TestSpinnerAnimatesInRunningTool (0.00s)
```

**Success Criteria**: Test MUST pass, indicating spinner now animates correctly.

---

## Phase 3: Update Existing Tests (Maintain GREEN)

### Step 3.1: Identify Affected Test Files

Run the full test suite to find compilation errors:

```bash
cd internal/ui
go test ./...
```

**Expected Errors**: Any test that creates `engine.Deps` manually will fail to compile because:
1. `ViewTool` field no longer exists (now `ToolRenderer`)
2. Field type changed from function to interface

**Likely affected files**:
- `internal/ui/engine/*_test.go`
- `internal/ui/compose/*_test.go`
- Any other test creating Deps

### Step 3.2: Fix Engine Tests

**For each test file in `internal/ui/engine/`**:

Create a mock ToolRenderer:

```go
type mockToolRenderer struct{}

func (m *mockToolRenderer) Render(t *engine.ToolState, spinner engine.SpinnerViewProvider) string {
	// Return simple mock output
	spinnerView := ""
	if spinner != nil {
		spinnerView = spinner.SpinnerView() + " "
	}
	return fmt.Sprintf("%s%v", spinnerView, t.Display)
}
```

Update any `Deps` creation:

```go
// OLD:
deps := engine.Deps{
	ViewTool: func(t *engine.ToolState) string { return "mock" },
}

// NEW:
deps := engine.Deps{
	ToolRenderer: &mockToolRenderer{},
}
```

### Step 3.3: Fix Compose Tests

**File**: `internal/ui/compose/deps_test.go` (if exists)

Update any tests that verify Deps creation:

```go
// Update assertions to check ToolRenderer instead of ViewTool
if deps.ToolRenderer == nil {
	t.Error("ToolRenderer should not be nil")
}
```

### Step 3.4: Run Full Test Suite

**Command**:
```bash
cd /Users/mac/Desktop/repos/iav
go test ./...
```

**Expected Result**: All tests pass (100% green).

**Success Criteria**: No compilation errors, no test failures.

---

## Phase 4: REFACTOR - Cleanup and Polish

### Step 4.1: Remove Dead Code

**File**: `internal/ui/compose/deps.go`

Verify the old `viewTool` function has been deleted (it's replaced by `toolRenderer.Render` method).

### Step 4.2: Update Documentation

**File**: `docs/contributing/ui_behaviour.md`

Add a note about the ToolRenderer interface (optional, but good for maintainability):

```markdown
### C. Tool Rendering

*   **Purpose:** Renders tool displays (Read, Edit, Shell, etc.) in the pending view.
*   **Implementation:** `ToolRenderer` interface defined in `engine/contracts.go`, implemented by `compose/deps.go`.
*   **Behavior:**
    *   Receives `ToolState` (tool data) and `SpinnerViewProvider` (current spinner frame).
    *   For Running tools: displays animated spinner prefix.
    *   For Success tools: displays checkmark (✓).
    *   For Error tools: displays X (✗).
```

### Step 4.3: Verify Test Coverage

**Command**:
```bash
cd internal/ui/tea
go test -cover -v
```

**Expected**: Coverage should include the new `TestSpinnerAnimatesInRunningTool`.

---

## Phase 5: Manual Verification

### Step 5.1: Build the Application

```bash
cd /Users/mac/Desktop/repos/iav
go build -o ./bin/iav ./cmd/iav
```

**Expected**: Successful build with no errors.

### Step 5.2: Test with Running Tool

**Scenario**: Execute a long-running command to observe the spinner.

```bash
./bin/iav "read a large file and summarize it"
```

or

```bash
./bin/iav "run shell command: sleep 5"
```

**Expected Behavior**:
- ✅ Tool display box shows a spinner prefix (e.g., `⠋`, `⠙`, `⠹`, `⠸`, `⠼`, `⠴`, `⠦`, `⠧`, `⠇`, `⠏`)
- ✅ The spinner character **changes** over time (animates)
- ✅ The spinner does NOT stay frozen on one character
- ✅ After tool completes, spinner changes to checkmark (✓) or X (✗)

### Step 5.3: Test Static UI Scenario

**Scenario**: Create a situation where no new events are coming in but a tool is running.

1. Start a command that runs a tool
2. Observe that only spinner ticks are occurring (no text streaming)
3. Verify spinner still animates

**Expected Behavior**: Spinner animates even when UI is otherwise static (no markdown streaming, no new tools).

---

## Rollback Plan

If the fix introduces regressions:

### Quick Rollback Steps

1. **Revert commits**:
   ```bash
   git revert HEAD
   ```

2. **Or manually restore**:
   - Restore `engine/types.go` (change `ToolRenderer` back to `ViewTool func`)
   - Restore `engine/transition.go` (change call sites back)
   - Restore `compose/deps.go` (restore old `viewTool` function and closure)
   - Restore `compose/compose.go` (restore spinner closure in factory)
   - Delete the new test

3. **Verify rollback**:
   ```bash
   go test ./...
   go build ./cmd/iav
   ```

---

## Success Criteria Summary

### Must Pass:
- ✅ New test `TestSpinnerAnimatesInRunningTool` passes
- ✅ All existing tests pass (no regressions)
- ✅ Application builds successfully
- ✅ Manual testing shows spinner animating in tool displays
- ✅ Manual testing shows spinner animates even in static UI

### Code Quality:
- ✅ No compilation errors or warnings
- ✅ Follows existing architectural patterns (interface-based DI)
- ✅ Code is cleaner (no stale closures)
- ✅ Consistent with design guidelines in `docs/contributing/design.md`

---

## Estimated Time

- **Phase 1 (Write Test)**: 30 minutes
- **Phase 2 (Implement Fix)**: 45 minutes
- **Phase 3 (Update Tests)**: 30 minutes
- **Phase 4 (Refactor)**: 15 minutes
- **Phase 5 (Manual Verification)**: 15 minutes

**Total**: ~2.5 hours

---

## Notes

- This fix aligns with the existing architecture where `deps.Spinner` is "runtime provided"
- The ToolRenderer interface follows the same pattern as other Deps interfaces (MarkdownStream, ThemeAdapter, LayoutAdapter)
- The fix eliminates closure capture bugs and makes the dependency explicit
- Future extensions (e.g., adding more methods to ToolRenderer) will be easier with the interface-based approach

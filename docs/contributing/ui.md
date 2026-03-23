# ui Package

## 1) Purpose

The `ui` package renders workflow state to the terminal and captures user actions during interactive commands.

## 2) Scope

### Owns
- Terminal rendering for prompt/history/auth/info/picker views.
- Event-driven view models that consume `domain.UIUpdate`.
- User action capture and forwarding as `domain.Action`.

### Does NOT own
- Workflow orchestration and decision logic.
- Tool or LLM execution.
- Session/state persistence.

## 3) Public Contract

### Inputs
- `domain.UIUpdate` from workflow/event bus.
- Terminal input (`tea.KeyMsg`, `tea.WindowSizeMsg`).

### Outputs
- Rendered terminal frames and flushed text.
- `domain.Action` to workflow (primarily `StopAction`).

### Invariants
- UI does not decide business flow.
- UI preserves chronological rendering order for stream/tool updates.
- `DoneEvent` leads to clean terminal completion.

## 4) Runtime Behavior

1. Poll bus updates and convert to UI messages.
2. Update model state (thinking/streaming/tooling/history/picker states).
3. Render current view and flush stable output when appropriate.
4. On `StopAction` request, cancel quickly and quit responsively.

Cancellation:
- User cancellation should be responsive and non-blocking from a UX perspective.

Concurrency:
- Bubble Tea model updates are sequential; bus polling and tick commands drive message flow.

## 5) Error Handling Policy

UI should internalize recoverable display issues and only fail hard when the view cannot continue safely.

| Scenario | Internalize? | Return error? | Typical action |
| --- | --- | --- | --- |
| Recoverable render/format issue | Yes | No | Fallback output and continue |
| Missing optional display metadata | Yes | No | Skip part and continue |
| Unexpected bus close | No | Yes | Show visible error line and terminate |
| Context/user cancellation | No | Yes (`ctx.Err` path upstream) | Request stop and quit |
| Invariant violation in UI state machine | No | Yes | Abort cleanly |

## 6) Dependencies

### Depends on
- `domain` for events/actions/types.
- `bubbletea`, `lipgloss`, markdown rendering utilities.

### Must NOT depend on
- `workflow` package logic.
- `cmd` wiring logic.
- Internal tool/LLM/service orchestration behavior.

## 7) Testing Expectations

- Model behavior is tested via concrete `tea.Msg` updates.
- Snapshot/golden tests should be updated only for intentional visual changes.
- Tests should assert event ordering, flush behavior, and cancellation semantics.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [architecture.md](architecture.md)
- [ui_behaviour.md](ui_behaviour.md)
- [testing.md](testing.md)
- [design.md](design.md)

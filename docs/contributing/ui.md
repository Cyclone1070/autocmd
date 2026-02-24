# ui Package

## Responsibility

The `ui` package is the presentation layer of the application. It provides an interactive command-line rendering engine that integrates plain-text flushing for static history and Bubble Tea for dynamic, currently-streaming content.

**Owns:**
- `Model` (`engine.go`): The main Bubble Tea model driving the presentation loop.
- `Stream` (`stream.go`): Accumulating LLM streaming text, identifying safe blocks via `yuin/goldmark`, and rendering markdown syntax (via `glamour`).
- Tool displays (`tool.go`): Formatting and coloring tool inputs, diffs, shell outputs, and results.
- Viewport dynamics (`truncate.go`): Enforcing layout and height constraints to safeguard the terminal buffer.

**Does NOT own:**
- Event generation or orchestration logic (delegated to `workflow` and `toolexecutor`).
- Domain entity interfaces or data types (delegated to `domain`).
- Core terminal lifecycle and application setup (delegated to `main.go` and its configuration logic).

---

## Event Handling Contract

The `ui` engine acts as a consumer of `domain.Event` messages dispatched by the workflow over a go channel. It processes events sequentially while concurrently managing simulated streaming text patterns.

### Event Processing Summary

| Event Type       | Render Effect                                                                  |
| ---------------- | ------------------------------------------------------------------------------ |
| `ThinkingEvent`  | Renders a spinner natively alongside the current thought process or status.    |
| `TextEvent`      | Defers content to the internal text queue to simulate a typing effect.         |
| `ToolStartEvent` | Flushes preceding completed blocks and adds the tool to the pending view.      |
| `ToolEndEvent`   | Updates tool status (success/error), captures final output, and flushes it.    |
| `DoneEvent`      | Flushes all remaining text and tool states to history, marks loop as complete. |
| `CancelEvent`    | Gracefully interrupts rendering, prints cancellation state, and triggers exit. |

### Internal Queuing and Flushing Requirements

To prevent blocking the event channel while preserving visual continuity (like typewriter effects for text streaming), the UI decouples ingestion from display via `tea.Cmd`:
1. **Event Ingestion**: Continuously polls the channel via `waitForEvent()`.
2. **Text Processing**: `TextEvent` data is chunked and stored in a queue.
3. **Stream Tick**: A recurring `streamTickMsg` drains the queue chunk-by-chunk, appending it to the `Stream`.
4. **Visibility Guarantees**: Tool boxes must always be flushed to history before pending text to strictly preserve vertical ordering/visual hierarchy. 
5. **Safe Flushing**: Only structurally complete "safe" chunks of Markdown are flushed fully out of the Bubble Tea view to standard output history. 

---

## Error Handling Contract

The UI package treats its ingestion channel and rendering responsibilities as **display-only**. 

### Rendering and System Failures

| Scenario                   | Loop Effect                                                                                    |
| -------------------------- | ---------------------------------------------------------------------------------------------- |
| **Markdown Parser Error**  | **Continues** — Falls back to unstyled raw text rendering in the stream.                       |
| **Unknown `domain.Event`** | **Continues** — Logged or ignored; the rendering loop proceeds to the next event continuously. |
| **Terminal Resize**        | **Continues** — Automatically intercepts `tea.WindowSizeMsg` to adjust configured width.       |

### Context / User Cancellation

- **`Ctrl+C` (Keyboard Interrupt)**: Intercepted via `tea.KeyMsg`. The UI terminates active loops, signals `tea.Quit`, and cleanly flushes any final buffer to standard output to preserve scrollback history.

**Key Rule**: The `ui` package is explicitly designed to never panic on misformed textual content or fail the overarching application workflow. Event errors are mapped gracefully to screen states, prioritizing application continuity over strict parsing enforcement.

---

*See [UI Behavior & Architecture](ui_behaviour.md) for more comprehensive details on the safe block flushing strategy and terminal interactions.*

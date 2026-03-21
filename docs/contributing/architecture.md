# Architecture

This document describes the high-level architecture of the application — its layers, their responsibilities, and the rules governing how they interact.

---

## Layers

```
┌─────────────────────────────────────────────────────────────┐
│                          cmd/                               │
│                                                             │
│  Wiring layer. Creates concrete instances, connects them    │
│  via dependency injection, and manages lifecycle.           │
│  No testable app logic lives here.                          │
│                                                             │
│  Can import: everything                                     │
└────────┬──────────────────┬──────────────────┬──────────────┘
         │ injects          │ injects          │ injects
         ▼                  ▼                  ▼
┌──────────────┐      ┌───────────────┐   ┌────────────────────┐
│  workflow/   │      │    ui/        │   │  internal services │
│              │      │               │   │                    │
│ Orchestrator │      │  Reactive     │   │  agent/  auth/     │
│ coordinates  │◄────►│  event-driven │   │  config/ fs/       │
│ use cases    │events│  display      │   │  llm/    session/  │
│              │      │               │   │  state/  tool/     │
│ imports:     │      │  imports:     │   │                    │
│ domain only  │      │  domain only  │   │  imports:          │
└──────────────┘      └───────────────┘   │  domain only       │
         │                  │             └────────────────────┘
         │                  │                    │
         ▼                  ▼                    ▼
    ┌─────────────────────────────────────────────────┐
    │                  domain/                        │
    │                                                 │
    │  Shared vocabulary. Pure types, events, and     │
    │  constants. Zero logic, zero dependencies.      │
    │                                                 │
    │  Can be imported by: everyone                   │
    └─────────────────────────────────────────────────┘
```

> [!NOTE]
> **Helper Packages Exception** — "Pure helper" packages (stateless utilities as defined in [design.md](design.md)) are an exception to the layer rules above. They can be imported directly by any layer since they introduce no stateful coupling or side effects.

---

## Layer Rules

### `cmd/` — Wiring

The entry point. Its only job is to:

1. Parse CLI flags and build configuration.
2. Pair a specific **workflow orchestrator** (e.g., `RunAuth`) with its corresponding **UI model** (e.g., `authui.Model`).
3. Wire them together via a dedicated, independent `EventBus`.
4. Manage lifecycle (`defer bus.Close()`, wait for workflow completion).

If you find yourself writing an `if` that isn't about flag parsing or wiring, it belongs in `workflow/` or an internal service.

> **Import rule:** Can import all other layers.

---

### `workflow/` — Orchestrator

Coordinates internal services to perform a use case (e.g., "run a prompt"). It does not create its own dependencies — everything is injected by `cmd/`.

- Accepts dependencies as **consumer-defined interfaces** (see [design.md](design.md)).
- Communicates with the UI exclusively through **events**, not direct calls.
- Sends `UIUpdate` events to the UI (text, tool status, done signal).
- Receives `Action` events from the UI (e.g., stop request).

> **Import rule:** `domain/` only. Does **not** import `ui/`, `cmd/`, or internal services.

---

### `ui/` — Display

Purely reactive. The UI's entire job is:

1. **React** to events from the workflow (render text, show spinners, display tool boxes).
2. **Send** user actions back to the workflow (e.g., Ctrl+C → `StopAction`).
3. **Quit** when the workflow says it's done (`DoneEvent`).

It contains zero application flow logic. It doesn't know what a "session" is, it doesn't know how LLMs work, it doesn't decide what happens next. It renders what it's told to render.

> **Import rule:** `domain/` only. Does **not** import `workflow/`, `cmd/`, or internal services.

---

### `internal/` services — Building Blocks

Small, independent packages that each do one thing:

| Package    | Purpose                                  |
|------------|------------------------------------------|
| `agent/`   | Runs the LLM agent loop (stream + tools) |
| `auth/`    | API key management                       |
| `config/`   | Configuration loading and defaults       |
| `eventbus/` | Async, buffered UI/Workflow communication |
| `fs/`       | Filesystem abstraction                   |
| `llm/`     | LLM provider adapters                    |
| `session/` | Session persistence (chat history)       |
| `state/`   | Runtime state (current session, etc.)    |
| `tool/`    | Tool declarations and implementations    |

> **Import rule:** `domain/` only. Do **not** import each other, `workflow/`, `ui/`, or `cmd/`.

---

### `domain/` — Shared Vocabulary

The one package everyone imports. Contains:

- **Event types** — `TextEvent`, `DoneEvent`, `ToolStartEvent`, etc.
- **Core types** — `Session`, `Message`, `Stream`, etc.
- **Constants** — `AppName`, `DefaultFilePerm`, etc.

It has **zero logic and zero dependencies**. If you're writing a function in `domain/`, it probably belongs somewhere else.

---

## Event Flow

The workflow and UI communicate through a bidirectional event bus:

```
  Workflow                    EventBus                    UI
     │                           │                        │
     │── SendUIUpdate ─────────► │                        │
     │                           │── eventMsg ──────────► │
     │                           │                        │── render text
     │                           │                        │
     │                           │                        │── User input 
     │                           │◄──────── SendAction ───│
     │◄──────── SendAction ──────│                        │
     │                           │                        │
     │                           │                        │
     │── SendUIUpdate(Done) ──►  │                        │
     │                           │── eventMsg ──────────► │
     │                           │                        │── flush + quit
     │                           │                        │
```

**Termination Contract:**

- **Natural Completion (Success Path)**: The workflow sends a `DoneEvent` as its final acknowledgement. The UI **waits** for this signal before printing success messages (e.g., "Authorized [Provider]") and quitting, ensuring all background state persistence (saves) has finished.
- **User Cancellation (Quit Path)**: When the user initiates a quit (`ctrl+c`, `esc`, `q`), the UI sends a `StopAction` and returns `tea.Quit` **immediately** to ensure the interface feels snappy. The workflow terminates independently upon receiving the action or when the `cmd` layer closes the bus.

**Polling Contract:**

- **Prompt (`internal/ui/prompt`)**: Uses **two independent loops**: `tea.Tick` drives animation (spinner, streaming chunks) and only `Init` / `handleTick` schedule the next tick; **`pollBus()`** (blocking on `UIUpdates()`) delivers bus events as messages. Polling is resumed from `handleFlushDone` and `tryResumePoll` (after `animatorDrainedMsg`) when appropriate — never batch `pollBus` with `nextTick` in the same command.
- **Reactive polling**: Other stationary components (e.g. `auth`, `pickers`) use blocking `pollBus()` commands that wait exclusively for the next event to save resources.


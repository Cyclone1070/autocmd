# workflow Package

## 1) Purpose

The `workflow` package orchestrates command-level use cases by coordinating dependencies and emitting UI updates.

## 2) Scope

### Owns
- Use-case orchestration (`RunPrompt`, `RunHistory`, picker/info/auth flows).
- Dependency coordination through consumer-defined interfaces.
- UI update emission and action consumption through bus interfaces.

### Does NOT own
- Terminal rendering.
- Tool implementation internals.
- Provider-specific LLM adapter internals.

## 3) Public Contract

### Inputs
- Command input (prompt text or command context).
- Dependency interfaces (store/state/agent/registry/bus).
- `domain.Action` events from UI bus.

### Outputs
- `domain.UIUpdate` events to UI.
- Workflow completion errors/results.

### Invariants
- Workflow never performs UI rendering directly.
- Workflow should emit terminal completion updates (`DoneEvent`) for UI-facing runs.
- Dependency creation remains in wiring layer (`cmd`).

## 4) Runtime Behavior

1. Resolve command context (session/state/model/provider as needed).
2. Execute use case until completion or cancellation.
3. Persist resulting state/session best-effort where applicable.
4. Emit final UI updates and return completion status.

Cancellation:
- `StopAction` must propagate to active context cancellation quickly.
- Workflows that participate in coordinated UI shutdown must emit a terminal `DoneEvent` after handling `StopAction`, so UIs can exit deterministically.

Concurrency:
- Workflow run is command-scoped and one-shot.
- Goroutines may be used for asynchronous waiting/bridging, but completion is still single-run.

## 5) Error Handling Policy

Workflow should internalize recoverable operational failures when useful for self-correction, and return errors for terminal failures.

| Scenario | Internalize? | Return error? | Typical action |
| --- | --- | --- | --- |
| Tool failure that model can self-correct | Yes | No | Convert to conversation-visible message |
| Non-critical metadata/load fallback | Yes | No | Continue with fallback/default |
| Context cancellation | No | Yes (`ctx.Err`) | Abort run |
| LLM/provider hard failure for active turn | No | Yes | Abort run and surface error |
| Session/state dependency failure | No | Yes | Abort run |
| Invariant violation | No | Yes | Abort run |

## 6) Dependencies

### Depends on
- `domain` vocabulary.
- Consumer-defined interfaces over internal services.

### Must NOT depend on
- UI package internals.
- cmd wiring concerns.

## 7) Testing Expectations

- Unit tests should mock dependency interfaces locally.
- Tests should cover cancellation, done-event emission, and persistence interactions.
- Tool error continuation vs terminal failure paths must be explicit in tests.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [architecture.md](architecture.md)
- [toolexecutor.md](toolexecutor.md)
- [testing.md](testing.md)
- [design.md](design.md)

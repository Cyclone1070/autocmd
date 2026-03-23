# tool Package

## 1) Purpose

The `tool` package provides concrete tools and registry wiring used by the agent/tool execution path.

## 2) Scope

### Owns
- Tool registry implementation and declaration exposure.
- Tool domains (`file`, `directory`, `search`, `shell`, `todo`).
- Tool prepare/execute contracts at package boundaries.

### Does NOT own
- Workflow orchestration and loop control.
- UI rendering logic.
- Provider-backed LLM logic.

## 3) Public Contract

### Inputs
- Tool invocations from executor/agent paths.
- Invocation arguments and execution context.

### Outputs
- Prepared invocations from `Prepare`.
- Execution content and execution errors from `Execute`.
- Tool declarations for model exposure.

### Invariants
- `Prepare` validates and normalizes inputs before execution.
- `Execute` produces content suitable for model feedback.
- Context cancellation is respected cooperatively.

## 4) Runtime Behavior

1. Registry resolves tool by name.
2. Tool validates arguments in `Prepare`.
3. Invocation executes in `Execute`.
4. Executor decides continue-vs-terminate policy using return values.

Cancellation:
- `Execute` must check context and return promptly on cancellation.
- `Prepare` should check context around potentially expensive I/O or external calls.

Concurrency:
- Tool implementations should avoid hidden global mutable state.

## 5) Error Handling Policy

Tool methods surface validation/operation errors; the executor decides whether those errors are loop-fatal or converted into model-visible messages.

| Scenario | Internalize in tool? | Return error from tool? | Typical action |
| --- | --- | --- | --- |
| Input validation failure in `Prepare` (ctx alive) | No | Yes | Return validation error; executor converts to model-visible message |
| Execution operation failure (ctx alive) | Usually no | Often yes with error content | Return `(errorContent, err)`; executor typically continues loop |
| Optional field/path missing where fallback exists | Yes | No | Apply fallback/default and continue |
| Context cancellation | No | Yes (`ctx.Err`) | Abort current execution |
| Invariant violation / unrecoverable corruption risk | No | Yes | Abort and surface error |

## 6) Dependencies

### Depends on
- `domain` tool interfaces/types.
- Tool-local helpers/services as needed.

### Must NOT depend on
- UI package internals.
- Workflow command flow logic.

## 7) Testing Expectations

- Each tool package has focused unit tests for prepare/execute behavior.
- Cancellation behavior should be explicitly tested.
- Validation and failure-message shaping should be deterministic.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [toolexecutor.md](toolexecutor.md)
- [architecture.md](architecture.md)
- [testing.md](testing.md)
- [design.md](design.md)

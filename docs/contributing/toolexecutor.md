# Internal: toolExecutor

## 1) Purpose

`toolExecutor` bridges model tool calls to concrete tool implementations and converts results into loop-consumable messages.

## 2) Scope

### Owns
- Tool lookup from registry.
- `Prepare` then `Execute` lifecycle sequencing.
- Tool progress/result UI events (`ToolStartEvent`, `ToolStreamEvent`, `ToolEndEvent`).
- Conversion of outcomes to conversation-visible messages.

### Does NOT own
- Individual tool implementation internals.
- Main loop control policy.
- UI rendering.

## 3) Public Contract

### Inputs
- Tool call requests (name + arguments).
- Registry/dependency interfaces.
- Execution context.

### Outputs
- Conversation-ready tool result messages.
- Fatal errors only when loop should stop.

### Invariants
- Exactly one tool lifecycle per resolved call.
- Non-fatal tool failures remain model-visible.
- Fatal errors are explicit and stop-capable.

## 4) Runtime Behavior

1. Resolve tool by name.
2. Execute `Prepare`.
3. Execute prepared invocation if valid.
4. Emit tool lifecycle updates.
5. Return formatted message or fatal error.

Cancellation:
- `ctx.Err()` must terminate promptly.
- On cancellation, executor must still emit a `ToolEndEvent` for any tool that started, using the tool-provided `finalDisplay` (which should surface cancellation via `GetError()`, typically `domain.ToolErrorCancelled`).

Streaming:
- For streamable tools, executor may forward `ToolStreamEvent` chunks while the tool runs. Regardless of streaming, a single `ToolEndEvent` terminates the tool lifecycle.

Concurrency:
- Executor is run-scoped and should avoid hidden cross-run state.

## 5) Error Handling Policy

Executor should internalize recoverable tool failures into returned messages and only return fatal errors for stop conditions.

| Scenario | Internalize? | Return error? | Typical action |
| --- | --- | --- | --- |
| Tool not found | Yes | No | Return message listing/clarifying available tools |
| `Prepare` validation failure | Yes | No | Return schema/validation message |
| `Execute` operation failure | Yes | No | Return error content message |
| Context cancellation | No | Yes (`ctx.Err`) | Abort loop |
| Unexpected executor invariant failure | No | Yes | Abort loop |

Additional invariant:
- `Invocation.Execute(ctx)` must return a non-nil `finalDisplay` on all paths (including cancellation). The executor treats a nil `finalDisplay` as a programmer error.

## 6) Dependencies

### Depends on
- `domain` tool/event vocabulary.
- Registry and tool interfaces.

### Must NOT depend on
- UI rendering package internals.
- Command wiring concerns.

## 7) Testing Expectations

- Tests should assert lifecycle event order and message shaping.
- Continue-loop vs terminate-loop paths must be explicit.
- Throughput/batching behavior should remain deterministic under test harness.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [tool.md](tool.md)
- [workflow.md](workflow.md)
- [testing.md](testing.md)
- [design.md](design.md)

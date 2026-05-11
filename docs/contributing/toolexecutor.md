# Internal: ReAct tool execution

## 1) Purpose

Production execution uses `react.Agent` + `compose.ToolsNode` middleware. Tool execution is no longer driven by a bespoke batch executor in the main prompt path.

## 2) Scope

### Owns
- Tool lookup from registry.
- Option B preflight in middleware (`Prepare` then permission gate before tool run).
- Tool progress/result UI events (`ToolStartEvent`, `ToolStreamEvent`, `ToolEndEvent`).
- Conversion of outcomes to conversation-visible messages.

### Does NOT own
- Individual tool implementation internals.
- UI rendering.

## 3) Public Contract

### Inputs
- Tool call requests (name + arguments).
- Registry/dependency interfaces.
- Execution context (event sender, action waiter, display sink).

### Outputs
- Conversation-ready tool result messages.
- UI lifecycle events.

### Invariants
- Exactly one tool lifecycle per resolved call.
- Non-fatal tool failures remain model-visible.
- Tools execute sequentially per assistant turn (`ToolsNodeConfig.ExecuteSequentially=true`).

## 4) Runtime behavior

1. Resolve tool by name in middleware.
2. Run `Prepare` preflight in middleware.
3. Emit `ToolStartEvent`.
4. If permission mode is `ask`, emit approval request and block for decision.
5. Invoke tool endpoint (interactive or executable path).
6. For streamable invocations, forward `ToolStreamEvent` chunks while executing.
7. Emit `ToolEndEvent` with final display and persist display by call ID.

Cancellation:
- `ctx.Err()` must terminate promptly.
- A started tool must still emit `ToolEndEvent` with cancellation reflected in `ToolDisplay.Error`.

Streaming:
- For streamable tools, middleware/adapter forwards `ToolStreamEvent` chunks while the tool runs.
- A single `ToolEndEvent` terminates the tool lifecycle.

Concurrency:
- Tool calls are sequential at the ToolsNode level by explicit policy.

## 5) Error handling policy

Middleware/tool path should internalize recoverable tool failures into returned messages; only model stream/provider failures bubble out as run errors.

| Scenario | Internalize? | Return error? | Typical action |
| --- | --- | --- | --- |
| Tool not found | Yes | No | Return deterministic unknown-tool message |
| `Prepare` validation failure | Yes | No | Return schema/validation message |
| Permission denied | Yes | No | Return denial message and `ToolEndEvent` with permission error |
| Tool execution failure | Yes | No | Return error content message |
| Context cancellation | No | Yes (`ctx.Err`) | Abort run |
| Model backend/auth failure | No | Yes | Return classified model error |

Additional invariant:
- Invocation execution must return a non-nil final display on all paths (including cancellation).

## 6) Dependencies

### Depends on
- `react.Agent` / `compose.ToolsNode`.
- `domain` tool/event vocabulary.
- Registry and tool interfaces.

### Must NOT depend on
- UI rendering package internals.
- Command wiring concerns.

## 7) Testing expectations

- Tests should assert lifecycle event order and message shaping.
- Permission ask-mode behavior should be deterministic.
- Sequential tool execution behavior must remain explicit.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [tool.md](tool.md)
- [workflow.md](workflow.md)
- [testing.md](testing.md)
- [design.md](design.md)

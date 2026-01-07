# workflow/toolmanager Package

## Responsibility

The `toolmanager` package manages tool registry and execution. It translates between provider types and internal tool types.

**Owns:**
- Tool registry: maps tool names → tool implementations
- Calling `Tool.Prepare()` and `Invocation.Execute()`
- Event emission: `ToolStartEvent` and `ToolEndEvent`
- Response construction: returns `provider.Message` with LLM content

**Does NOT own:**
- Individual tool implementations (in `tool/` subpackages)
- Loop orchestration (that's `loop`)

---

## Error Handling Rules

### Context Cancellation

Context cancellation **always terminates the loop**. Check `ctx.Err()` after both `Prepare()` and `Execute()` return errors.

### Return Contract

| Scenario                      | Return           | Loop Effect                              |
| ----------------------------- | ---------------- | ---------------------------------------- |
| Tool not found                | `(message, nil)` | **Continues** — LLM sees available tools |
| Prepare error + ctx cancelled | `(_, error)`     | **Terminates**                           |
| Prepare error + ctx OK        | `(message, nil)` | **Continues** — LLM sees expected schema |
| Execute error + ctx cancelled | `(_, error)`     | **Terminates**                           |
| Execute error + ctx OK        | `(message, nil)` | **Continues** — LLM sees error content   |
| Execute success               | `(message, nil)` | **Continues** — LLM sees result          |

---

## Errors This Package Throws

`Execute()` returns an error **only** for context cancellation.

When the caller receives an error, stop iterating tool calls. The loop should terminate.

When the caller receives `(message, nil)`, add the message to conversation. The loop continues regardless of message content.

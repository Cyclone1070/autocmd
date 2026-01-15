# Internal: toolExecutor

## Responsibility

The `toolExecutor` is a private struct within the `workflow` package. It bridges the gap between the orchestration loop and the `toolRegistry`.

**Owns:**
- Calling `Tool.Prepare()` and `Invocation.Execute()` for tool calls
- Managing the mapping of tool names to implementations (via `toolRegistry`)
- Emitting tool-related events (`ThinkingEvent`)
- Formatting tool results into `domain.Message`

**Does NOT own:**
- Individual tool implementations (in `tool/` subpackages)
- The main decision loop (that's `Workflow.Run`)

---

## Error Handling Contract

The `toolExecutor` is designed to be resilient, absorbing most errors to allow the LLM to self-correct.

### Context Cancellation

Context cancellation **always terminates the loop**. The executor checks `ctx.Err()` after `Prepare()` and `Execute()`. If cancelled, it returns the cancellation error.

### Return Contract

| Scenario                   | Return Type      | Loop Effect                              |
| -------------------------- | ---------------- | ---------------------------------------- |
| **Tool Not Found**         | `(message, nil)` | **Continues** — LLM sees available tools |
| **Prepare Error** (ctx OK) | `(message, nil)` | **Continues** — LLM sees schema/reason   |
| **Execute Error** (ctx OK) | `(message, nil)` | **Continues** — LLM sees error output    |
| **Execute Success**        | `(message, nil)` | **Continues** — LLM sees output          |
| **Context Cancelled**      | `(_, ctx.Err())` | **Terminates**                           |

**Key Rule**: If the executor returns `nil` error, the loop **expects** a message to add to history. If it returns `non-nil` error, the loop **aborts**.

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

How to handle errors received from tools:

1. **Tool not found.** Return a message listing available tools. Do not return an error — let the LLM retry with a valid tool name.

2. **Prepare error.** Wrap the error in a message: `"Error: failed to prepare tool: <error>"`. Do not propagate the error. The LLM sees the message and can fix its request.

3. **Execute context error.** If `Execute()` returns an error and `ctx.Err() != nil`, propagate the error immediately. This terminates the loop.

4. **Execute tool error.** If `Execute()` returns `(llmContent, err)` where err is not a context error, wrap `llmContent` in a message and emit `ToolEndEvent` with the error flag set. Do not propagate the error — the loop continues.

5. **Execute success.** Wrap the content in a message and emit `ToolEndEvent` without error.

---

## Errors This Package Throws

`ToolManager.Execute()` returns an error in the following cases:

1. **Context cancellation.** User cancelled or timeout expired. This is the only error that propagates.

**What callers should do:**

When `Execute()` returns an error, stop iterating tool calls. The loop should terminate.

When `Execute()` returns `(message, nil)`, add the message to the conversation regardless of whether it contains error text. The loop continues.

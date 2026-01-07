# workflow/loop Package

## Responsibility

The `loop` package orchestrates the agent's think-act cycle. It coordinates between the LLM provider and tool manager.

**Owns:**
- Main agent loop: send messages → get response → handle tool calls → repeat
- Coordination between `llmProvider` and `toolManager` interfaces
- Emitting loop-level events: `ThinkingEvent`, `TextEvent`, `DoneEvent`

**Does NOT own:**
- Tool registry or parsing (delegated to `toolmanager`)
- LLM communication details (delegated to `provider`)
- Tool-specific events (emitted by `toolmanager`)

---

## Error Handling Rules

### Loop-Terminating Errors

These errors stop the loop immediately:

1. **Provider error.** LLM returned an error (network, API, rate limit). No recovery path.
2. **Context cancellation.** User cancelled or timeout expired. Can come from:
   - Loop's own `ctx.Err()` check at iteration start
   - `provider.Generate()` returning an error
   - `toolmanager.Execute()` returning an error (from Prepare or Execute)

When the loop receives an error, it:
1. Adds `[Session cancelled by user]` to session (for context errors)
2. Saves session (best effort)
3. Returns the error to caller

### Loop-Continuing Messages

These are **not errors** — they are messages added to conversation:

1. **Tool returned error content.** The toolmanager wraps it in a message. Loop continues.
2. **Tool not found.** The toolmanager returns a message with available tools. Loop continues.
3. **Tool preparation failed.** The toolmanager returns a message with expected schema. Loop continues.

**Key rule:** If `toolmanager.Execute()` returns `(message, nil)`, the loop always continues — even if the message contains error text.

---

## Error Flow

```
Context cancelled       → toolmanager returns error → loop terminates
Provider error          → loop terminates
Tool validation error   → toolmanager returns message → LLM sees it → loop continues
Tool operation error    → toolmanager returns message → LLM sees it → loop continues
```

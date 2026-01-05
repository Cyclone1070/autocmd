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

How to handle errors received from dependencies:

1. **Provider error → terminate.** If the LLM provider returns an error, propagate it immediately. There is no recovery path — the agent cannot continue without LLM responses.

2. **ToolManager error → terminate.** The only error toolmanager returns is context cancellation. Propagate it to stop the loop.

3. **ToolManager success with error content → continue.** When toolmanager returns a message (even one containing error text), add it to the conversation and continue the loop. The agent will see the error and decide what to do next.

---

## Errors This Package Throws

`Loop.Run()` returns an error in the following cases:

1. **LLM provider failure.** Network errors, API errors, rate limits, malformed responses. The loop cannot continue without valid LLM responses.

2. **Context cancellation.** User cancelled, timeout expired, or parent context was cancelled. The loop respects context and exits cleanly.

**What callers should do:**

These errors are fatal to the current conversation. Log the error and notify the user. There is no automatic retry logic — the caller decides whether to start a new loop.

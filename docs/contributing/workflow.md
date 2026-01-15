# workflow Package

## Responsibility

The `workflow` package is the core orchestrator of the one-shot CLI application. It manages the agent's think-act cycle for a single execution.

**Owns:**
- Main agent loop: `Run()` (send messages → get response → handle tool calls → repeat)
- Internal `toolExecutor`: Handles tool execution and display logic
- Session management: Loading, updating, and saving the session state
- Event emission: `ThinkingEvent`, `TextEvent`, `DoneEvent`

**Does NOT own:**
- Tool implementations (in `tool/` subpackages)
- LLM communication details (delegated to `llm` package via `modelRegistry`)
- Persistence details (delegated to `session` package)

---

## One-Shot Architecture

The workflow operates as a **one-shot process**.
1.  **Start**: Loaded with a session and a model.
2.  **Run**: Executes the loop until completion, error, or cancellation.
3.  **Exit**: Saves state and returns.

There is no concurrent state sharing or long-running background service.

---

## Error Handling Rules

### Loop-Terminating Errors

These errors stop the loop immediately:

1.  **Context Cancellation**: User cancelled (Ctrl+C) or timeout.
    -   Action: Add `[Session cancelled by user]` to history, save session, return error.
2.  **Model Error**: LLM returned a fatal error (network, API, rate limit).
    -   Action: Save session, return error.
3.  **Max Iterations**: Loop exceeded the configured limit.
    -   Action: Add `[Max iterations reached]` to history, save session, return error.

### Loop-Continuing Messages

These are **not errors** — they are messages added to the conversation so the LLM can react:

1.  **Tool Error**: A tool failed to execute (e.g., file not found).
    -   Action: `toolExecutor` returns a formatted error message. Loop adds it to history and continues.
2.  **Tool Validation Error**: LLM sent invalid arguments.
    -   Action: `toolExecutor` returns a message with the schema. Loop continues.

---

## Internal: Tool Executor

The `toolExecutor` is a private helper within `workflow` that handles the details of:
-   Looking up tools in `toolRegistry`
-   Calling `Prepare()` and `Execute()`
-   Emitting tool events
-   Formatting results for the LLM

It is **not** a separate package.

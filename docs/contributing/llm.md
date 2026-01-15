# llm Package

## Responsibility

The `llm` package provides concrete implementations for LLM backends and the `Registry` that manages them.

**Owns:**
- `Registry`: Implementation of `domain.ModelRegistry`
- `google`: Concrete adapter for Google Gemini
- Resolution of model IDs (e.g., "google/gemini-pro") to `domain.Model` instances

**Does NOT own:**
- Core interfaces (`Model`, `ModelRegistry` in `domain`)
- Data types (`Message`, `ToolCall`, `Stream` in `domain`)
- Workflow orchestration

---

## Error Handling Contract

The `llm` package is the source of all generation errors. These errors are generally **fatal** to the current turn.

### Context Cancellation

Context cancellation **always terminates the loop**. The provider must check `ctx.Err()` and forward it.

### Return Contract

| Scenario                      | Return Type           | Loop Effect                             |
| ----------------------------- | --------------------- | --------------------------------------- |
| **API Error** (ComputeTokens) | `(0, error)`          | **Caller handles** (usually logs/exits) |
| **API Error** (Stream start)  | `(nil, error)`        | **Terminates**                          |
| **Streaming Error** (Next)    | `stream.Err() != nil` | **Terminates**                          |
| **Context Cancelled**         | `error`               | **Terminates**                          |

**Key Rule**: Any error returned by `Get()`, `List()`, `ComputeTokens()`, `Stream()`, or `stream.Err()` is considered a failure of the backend. The workflow will save the session and terminate.

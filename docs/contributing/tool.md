# tool Package

## Responsibility

The `tool` package defines tool types and display data structures. It is the foundational layer that other packages import for type definitions.

**Owns:**
- `Declaration` — Tool schema (name, description, parameters) sent to LLM
- `Schema` — JSON Schema type definitions for parameters
- `Invocation` — Interface for prepared tool calls
- `ToolDisplay` — Interface for UI display types
- `StringDisplay`, `DiffDisplay`, `ShellDisplay` — Concrete display types

**Does NOT own:**
- Tool execution logic (individual tools in subpackages)
- Tool registry or orchestration

---

## Error Handling Rules (for tool implementations)

### `Prepare()` Contract

**Returns error for:** Validation failures. Anything that can be determined before doing real work.
- Input parsing failures
- Missing or invalid parameters
- Path resolution failures

**Never returns error for:** I/O operations. Prepare must not perform file reads, network calls, or other side effects.

### `Execute()` Contract

**Returns `("", ctx.Err())` for:** Context cancellation. Check `ctx.Err()` before and after I/O operations.

**Returns `(errorContent, err)` for:** Operation failures. The error content describes the problem for the LLM. The error itself is for logging/metrics.
- File system errors (not found, permission denied, etc.)
- Network errors
- Validation errors discovered during execution (e.g., binary file detection)

**Returns `(content, nil)` for:** Success.

---

## Errors This Package Throws

### From `Prepare()`

All errors are validation failures. They should be wrapped in a message and shown to the LLM. Do not propagate to the loop.

### From `Execute()`

Two categories:

1. **Context errors.** Must propagate. Terminates the loop.

2. **Operation errors.** Must NOT propagate. The error content is embedded in the first return value (`llmContent`). The second return value (`err`) is for logging only.

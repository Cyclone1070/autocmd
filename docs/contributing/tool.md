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

**Responsibility:** Validate that the request is syntactically correct and semantically feasible.
**Returns error for:**
- Invalid inputs (parsing, missing fields).
- State violations (e.g. target does not exist).
*Validation should be comprehensive to "fail fast".*

### `Execute()` Contract

**Responsibility:** Perform the action safely.
**Requirement:** **Re-verify volatile state.** Because time passes between Prepare and Execute, valid state (like file existence) may have changed. You must re-check it to prevent race conditions.

**Returns `("", ctx.Err())` for:** Context error. Check `ctx.Err()` before/after I/O.

**Returns `(errorContent, err)` for:** Operation failures.
- File system errors
- Network errors
- Validation errors discovered during execution

**Returns `(content, nil)` for:** Success.

## Errors This Package Throws

### From `Prepare()`

All errors are validation failures. They should be wrapped in a message and shown to the LLM. Do not propagate to the loop.

### From `Execute()`

Two categories:

1. **Context errors.** Must propagate. Terminates the loop.

2. **Operation errors.** Must NOT propagate. The error content is embedded in the first return value (`llmContent`). The second return value (`err`) is for logging only.

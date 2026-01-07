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

## Error Handling Contract

### Context Cancellation

Context cancellation **always terminates the loop**. Both `Prepare()` and `Execute()` must check for it.

Go context is cooperative - functions must explicitly check `ctx.Err()`. Since `fs.Stat()` and similar I/O don't respect context, you must check after they return.

### `Prepare()` Returns

| Return                                  | When              | Loop Effect                             |
| --------------------------------------- | ----------------- | --------------------------------------- |
| `(invocation, nil)`                     | Validation passed | Continues to Execute                    |
| `(nil, error)` where `ctx.Err() != nil` | Context cancelled | **Loop terminates**                     |
| `(nil, error)` where `ctx.Err() == nil` | Validation failed | **Loop continues** — error shown to LLM |

### `Execute()` Returns

| Return                                         | When              | Loop Effect                                           |
| ---------------------------------------------- | ----------------- | ----------------------------------------------------- |
| `(content, nil)`                               | Success           | **Loop continues**                                    |
| `(errorContent, err)` where `ctx.Err() != nil` | Context cancelled | **Loop terminates**                                   |
| `(errorContent, err)` where `ctx.Err() == nil` | Operation failed  | **Loop continues** — content shown to LLM, err logged |

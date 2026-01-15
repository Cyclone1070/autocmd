# tool Package

## Responsibility

The `tool` package contains the concrete implementations of tools and the registry that holds them.

**Owns:**
- `Registry` — Implementation of the `toolRegistry` interface used by `workflow`.
- Subpackages for each tool area:
    - `file/` — File system operations (read, write, edit)
    - `directory/` — Directory listing
    - `search/` — File and content search
    - `shell/` — Command execution
    - `todo/` — Todo list management

**Does NOT own:**
- `Tool`, `Invocation`, `Declaration` interfaces (in `domain`)
- Workflow orchestration

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

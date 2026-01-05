# 5. Error Handling

> [!IMPORTANT]
> **Minimize Error Returns**
> 
> Every returned error forces the caller to handle it, adding complexity. Before returning an error, ask:
> * Can this be handled internally (clamp, default, fallback)?
> * Will my caller actually check this with `errors.Is` and handle it differently?
> * Is this truly exceptional, or just an edge case we can normalize?

**Goal**: Errors live with the code that returns them.

## Choosing Error Types

**Decision rule**: Does the caller programmatically check this error with `errors.Is`/`errors.As` and take a different action?

*   **YES → Sentinel or Struct**

    Use when the caller will branch on this error to do something different (retry, fallback, convert to different response, etc.).

    *   **Sentinel**: A named error value the caller checks with `errors.Is`. Used for simple errors.
        ```go
        var ErrNotFound = errors.New("not found")
        ```
    *   **Struct**: When the error is more complex and callers also need to extract context fields (path, code, etc.) using `errors.As`.
        ```go
        type PathError struct { Path string; Cause error }
        ```

*   **NO → `fmt.Errorf`**

    Use when the caller cannot programmatically handle this error — it just passes the error up to the user. The user (human or AI agent) sees the error message and fixes the issue externally.

    ```go
    return fmt.Errorf("stat %s: %w", path, err)
    ```

    This covers most errors: I/O failures, permission errors, network issues, unexpected errors.

> [!CAUTION]
> **FORBIDDEN PATTERNS**
>
> | Pattern | Why Bad |
> |---------|---------|
> | **Behavioral Interfaces** | `interface { NotFound() bool }` leads to boilerplate explosion. |
> | **Raw errors.New** | `return errors.New("fail")` is untestable. |
> | **Sentinel never checked** | If no caller uses `errors.Is`, use `fmt.Errorf` instead. |


> [!WARNING]
> **Sentinel Overuse is an Anti-Pattern**
> 
> Sentinels create coupling and become part of your public API. Use them sparingly — only when callers actually check with `errors.Is` and branch.

> [!TIP]
> **Merging Errors**: If multiple distinct errors lead to the same handling sequence, merge them into a single sentinel or use `fmt.Errorf` wrapping. Don't create separate error types just because the causes are different. Handling paths are what defines the error types.

**Example**:

```go
package file

// Sentinel - caller WILL check with errors.Is
var ErrNotFound = errors.New("file not found")

func (t *ReadTool) Run() error {
    // ...
    if !exists {
        return fmt.Errorf("%w: %s", ErrNotFound, path)
    }
    
    // OS error - caller WON'T check, just passes up
    content, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("read %s: %w", path, err)
    }
}
```

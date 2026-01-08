# Design Guidelines

> [!IMPORTANT]
> **Design Goals**: Prioritize high cohesion and testability.
>
> These principles are strict guidelines to ensure maintainable, testable, and robust Go code. Any violations, no matter how small, will be rejected immediately during code review.

---

## 1. Dependency Injection

**Goal**: Explicit, testable dependencies.

*   **Strict DI**: Dependencies MUST be passed via constructor.
    *   **Why**: Explicit dependencies make code testable and prevent hidden coupling.

*   **Pure Helpers vs Dependencies**:
    *   **Pure helpers**: Simple, static, standalone functions. These are often extracted from existing code for cross-package DRY compliant. Consumer should import directly, including interfaces, structs and errors. No local interface needed.
    *   **Dependencies**: Complex logic with struct for storing state. Define interface in consumer, inject via constructor, wiring in main.
    *   **Why**: DI is for swappable/mockable behavior. Pure functions are simple and static and don't need mocking.

*   **No Globals**: Never use global state for dependencies.
    *   **Why**: Globals create hidden dependencies, prevent parallel tests, and make code unpredictable.

---

## 2. Interfaces: Consumer-Defined

**Goal**: Decoupling and testability.

*   **Define Where Used**: Do NOT define interfaces in the implementing package. Define them in the consumer package.
    *   **Why**: The consumer knows what it needs. The implementer should not dictate the contract.
    *   **Benefit**: You can swap implementations without touching the consumer. You can mock easily in tests.

*   **Return Types From Provider**: Interface methods should return concrete types from the implementation package, not consumer-defined types.
    *   **Why**: Provider logically owns the return types. Defining them in provider is often simpler and offers high cohesion.

> [!CAUTION]
> **ANTI-PATTERN**: Copy Exact Interface Methods
>
> *   **Bad**: Consumer interface declares methods it never calls (e.g., `Rename()` exists but is never invoked).
> *   **Why**: The interface exposes internal implementation details or dependencies of dependencies.
> *   **How It Happens**: Copying methods from the implementer instead of auditing actual usage.
> *   **Solution**: Check your package for each interface method. If unused, remove it.


*   **No Shared Interfaces**: Interfaces are local to the package that uses them. NOT shared across packages, even siblings.
    *   **Why**: Sibling packages should not know each other exist. Each defines its own interface with only the methods IT needs.
    *   **Trade-off**: This creates duplication. You accept small duplication in exchange for more decoupling. This is correct.

> [!CAUTION]
> **ANTI-PATTERN**: Shared Interface Library
>
> *   **Bad**: Creating `internal/interfaces/filesystem.go` with a 10-method interface everyone imports.
> *   **Why**: This is `model/` in disguise. It couples all consumers and forces implementers to satisfy methods they don't need.
> *   **Solution**: Each consumer defines its own minimal interface. Duplication is acceptable. Coupling is not.

> [!TIP]
> **Exception: Helper Package Interfaces**
>
> If you already import a helper package (e.g., `pathutil`) and call its functions directly, you are already coupled to it. In this case, **import its interface directly** rather than redefining an identical interface locally. See [Pure Helpers](#1-dependency-injection) for more on this distinction.
>
> *   **Bad**: Redefining `type pathResolver interface { Lstat()... }` when you already import `path.Resolve`.
> *   **Good**: Use `path.FileSystem` directly since coupling already exists.
> *   **Why**: Redefining the interface is noise. It disguises where the requirement comes from.

---

## 3. Error Handling

> [!IMPORTANT]
> **Minimize Error Returns**
> 
> Every returned error forces the caller to handle it, adding complexity. Before returning an error, ask:
> * Can this be handled internally (clamp, default, fallback)?
> * Will my caller actually check this with `errors.Is` and handle it differently?
> * Is this truly exceptional, or just an edge case we can normalize?

**Goal**: Errors live with the code that returns them.

### Choosing Error Types

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

---

## 4. Testing

**Goal**: Deterministic, isolated, self-contained tests.

*   **Mocking**: Use mocks for all dependencies in unit tests.
    *   **Why**: Real dependencies (databases, filesystems) make tests slow, flaky, and non-deterministic.

*   **No Temp Files/Dirs**: Do not touch the filesystem in unit tests. Mock the interface.
    *   **Why**: Filesystem operations are slow and create test pollution across runs.

*   **Local Mocks**: Define mocks inside the `*_test.go` file that uses them. No shared `mock/` package.
    *   **Why**: Consumer-defined interfaces mean each test defines its own interface. The mock implements THAT interface. Mocks can't drift. No import cycles.

*   **Local Helpers**: Test helper functions should be defined in the test file that uses them.
    *   **Why**: Keeps tests self-contained and readable.

> [!CAUTION]
> **ANTI-PATTERN**: Shared Mock Package
>
> *   **Bad**: `internal/testing/mock/filesystem.go` with a "god mock" used everywhere.
> *   **Why**: Creates coupling, import cycles, and mocks that implement methods no single consumer needs.
> *   **Solution**: Define `mockFileSystem` inside `file/read_test.go` with only the methods `file.fileSystem` requires.

---

## Pre-Commit Checklist

Before submitting code, verify **every** item.

### Package Design
- [ ] Parent package does NOT import its sub-packages
- [ ] Types and errors live with their implementation package
- [ ] Shared types/errors in parent

### Dependency Injection
- [ ] Dependencies injected via constructor with consumer-defined interface
- [ ] Pure helpers imported directly (including their interfaces, structs, errors)

### Interfaces
- [ ] All interfaces defined in the **consumer** package, not the implementer
- [ ] Return types in interface methods are concrete types from **provider** package
- [ ] No unused methods in interfaces (grep to verify each method is called)

### Error Handling
- [ ] Sentinel/struct errors ONLY when caller checks with `errors.Is`/`errors.As`
- [ ] Use `fmt.Errorf` for all other errors (I/O, permissions, network, etc.)
- [ ] No behavioral interfaces (`NotFound() bool`), no raw `errors.New`

### Testing
- [ ] All mocks defined locally in `*_test.go` files (no shared `mock/` package)
- [ ] All test helpers defined locally in test files

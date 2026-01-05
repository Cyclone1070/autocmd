# 3. Interfaces: Consumer-Defined

**Goal**: Decoupling and testability.

*   **Define Where Used**: Do NOT define interfaces in the implementing package. Define them in the consumer package.
    *   **Why**: The consumer knows what it needs. The implementer should not dictate the contract.
    *   **Benefit**: You can swap implementations without touching the consumer. You can mock easily in tests.

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
> If you already import a helper package (e.g., `pathutil`) and call its functions directly, you are already coupled to it. In this case, **import its interface directly** rather than redefining an identical interface locally. See [Pure Helpers](02-dependency-injection.md) for more on this distinction.
>
> *   **Bad**: Redefining `type pathResolver interface { Lstat()... }` when you already import `path.Resolve`.
> *   **Good**: Use `path.FileSystem` directly since coupling already exists.
> *   **Why**: Redefining the interface is noise. It disguises where the requirement comes from.

**Example**:

```go
// package service - defines only what IT needs
type UserRepository interface {
    Find(id string) (*User, error)
}

type Service struct {
    repo UserRepository
}
```

**Sibling Isolation Example**:

```go
// package file - defines only what IT needs
type fileSystem interface {
    Stat(path string) (FileInfo, error)
    ReadFileRange(path string, offset, limit int64) ([]byte, error)
}

// package directory - different package, own interface
type fileSystem interface {
    Stat(path string) (FileInfo, error)
    ListDir(path string) ([]FileInfo, error)
}

// Both satisfied by the same OSFileSystem, but neither knows about the other.

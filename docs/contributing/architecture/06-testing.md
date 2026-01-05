# 6. Testing

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

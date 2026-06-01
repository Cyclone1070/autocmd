# Contributing to AutoCmd

Thank you for your interest in contributing to AutoCmd — a terminal-native AI assistant with file system and shell access.

This document covers the project's architecture, conventions, and workflows. It assumes you are familiar with Go.

---

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Project Architecture](#project-architecture)
- [Code Conventions](#code-conventions)
- [Testing Guidelines](#testing-guidelines)
- [Adding a New Tool](#adding-a-new-tool)
- [Pull Request Process](#pull-request-process)

---

## Getting Started

### Prerequisites

- **Go 1.26.2+** — see `go.mod` for the exact version.
- An LLM provider API key (Google Gemini or GitHub Models) for integration testing.

### Setup

```bash
git clone https://github.com/Cyclone1070/autocmd.git
cd autocmd
go mod download
```

### Build and Test

```bash
# Build all packages
go build ./...

# Run all tests (with race detection, as CI does)
go test -race ./...

# Run lint (must pass with zero warnings)
golangci-lint run --timeout=5m

# Run tests with coverage reporting
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## Development Workflow

### Strict TDD

This project follows **test-driven development** for all behavior changes and refactors. The rule is simple:

> Write the failing test **first**, then implement. No production code without a preceding failing test.

**Exceptions:**

- Purely mechanical edits (rename across repo, formatting) when behavior is unchanged.
- The `cmd/` layer is simple wiring — never write tests in `cmd/`.
- Doc-only or config-comment-only changes.

**File organization:** Every implementation file has exactly one corresponding test file:

```
internal/tool/read/read.go        → internal/tool/read/read_test.go
internal/agent/chat_turn.go       → internal/agent/chat_turn_test.go
```

No topic-based test files. If you are testing `implementation.go`, it goes in `implementation_test.go`.

### The Cycle

1. **Red** — Add or extend a test describing the desired behavior. It must fail on the current code, proving the test guards the change.
2. **Green** — Write the smallest amount of production code that makes the test pass.
3. **Refactor** — Improve structure with all tests still passing.

If something is hard to test, **narrow the design** — extract pure functions, define small interfaces, inject dependencies — until you can write the test first.

---

## Project Architecture

### Module Map

```
main.go                      — Entry point, delegates to cmd.Execute()
cmd/                         — Cobra CLI wiring + dependency injection
internal/
  domain/                    — Shared types, events, actions (zero dependencies)
  agent/                     — LLM agent loop (graph-runner on Eino compose.Graph)
  workflow/                  — Business logic orchestration (one goroutine per use case)
  eventbus/                  — In-process bidirectional event bus (UI ↔ Workflow)
  actionrouter/              — CallID-based action routing (permission approval, questions)
  ui/                        — Bubbletea TUI components
  tool/                      — Tool registry and implementations
    bash/                    — Shell command execution
    read/                    — File reading with checksum tracking
    write/                   — File writing
    edit/                    — File editing with diff display
    glob/                    — Pattern-based file search
    grep/                    — Content search
    question/                — Interactive user questions
    save/                    — Command saving
    mcp/                     — MCP external tool adapter
    service/                 — Shared tool services (executor, checksum, path)
  config/                    — JSON config loading, validation, defaults
  provider/                  — LLM provider registry (Google, GitHub)
  auth/                      — Credential management (file-based, OAuth device flow)
  permission/                — Tool permission resolver (ask/allow/deny)
  session/                   — Session persistence and CRUD
  state/                     — App state persistence (model selection)
  command/                   — Saved command persistence
  fs/                        — Filesystem abstraction (atomic writes, binary detection)
  logging/                   — Structured logging (slog → file)
  randutil/                  — Cryptographically random short IDs
  runtimectx/                — Context-value DI for tools
```

### Dependency Flow

Dependencies flow **inward**. The `domain/` package has zero dependencies on the rest of the codebase:

```
cmd/ → internal/* → domain/
```

Key import rules enforced by `internal/architecture_test.go` (via `go-arctest`):

| Package | May import | Must NOT import |
|---------|-----------|----------------|
| `domain/` | stdlib + Eino schema | Anything else |
| `workflow/` | `domain/` only | `ui/`, `cmd/`, services directly |
| `ui/` | `domain/`, `eventbus` | `workflow/`, `agent/`, services |
| Services | `domain/`, `eventbus` | `workflow/`, `ui/`, `cmd/` |
| `cmd/` | Everything (wiring) | - |

### Key Design Patterns

**Graph-runner state machine** — The agent loop (`internal/agent/`) uses Eino's `compose.Graph` with four lambda nodes (`preturn → chat → run_tools → preturn`) orchestrated as a cyclic directed graph, not a monolithic loop.

**Event-driven UI** — The agent never imports Bubbletea code. It emits typed events over an in-process bus (`internal/eventbus/`). The UI subscribes independently. Workflows and views communicate exclusively through events and actions.

**Consumer-defined interfaces** — Following Dependency Inversion, each consumer defines its own minimal interface. Workflow defines interfaces for exactly what it needs (`sessionStore`, `agentRunner`, `bus`). Implementations are injected at the `cmd/` wiring layer.

**Tool abstraction** — Every tool implements Eino's `BaseTool` interface. Extended interfaces (`Preview`, `PreflightValidate`, `IsConcurrentSafe`) are checked at runtime. Tools never import TUI code — they access context dependencies via `runtimectx` package.

---

## Code Conventions

### Naming

- **Files**: Lowercase with underscores for compound names (`session_picker.go`).
- **Test files**: `implementation.go` → `implementation_test.go` (one-to-one).
- **Test helper constructors**: Prefixed with `new` (`newMockFileSystem`).
- **Test function names**: `Test<FunctionName>_<Scenario>` (`TestReadFile_LargeInput`).
- **Test constants**: Prefixed with `test` (`testToolNameGreet`).

### Dependency Injection

- Dependencies must be passed via constructor (not global state, not `init()`).
- Pure helper functions (stateless, no mocking needed) can be imported directly.
- All other dependencies use consumer-defined interfaces injected via constructor.

### Interfaces

- Define interfaces in the **consumer** package, not the implementer.
- Interface methods should return concrete types from the provider package (not consumer-defined types).
- No unused methods in interfaces — grep to verify each is called.
- No shared interface packages — duplication is acceptable, coupling is not.

### Error Handling

- Use `errors.New` / `fmt.Errorf` with descriptive messages.
- Define sentinel errors only when callers actually check with `errors.Is`.
- For structured errors with context fields, define error structs checked with `errors.As`.
- Wrap errors with context: `fmt.Errorf("read %s: %w", path, err)`.

### Configuration

- Technical parameters (limits, timeouts) are internal constants, not user config.
- User-facing configuration lives in `~/.config/autocmd/config.json`.
- All defaults are defined in `internal/config/defaults.go`.

---

## Testing Guidelines

### Test Structure

- **One test file per implementation file.** No exceptions.
- **Table-driven tests** are preferred for data-driven scenarios.
- **`go test -race ./...`** must pass before any PR.
- **`golangci-lint`** must pass with zero warnings.

### Mock Patterns

This project uses two mock styles:

1. **Hand-rolled mocks** (preferred) — defined as unexported types in the `*_test.go` file that uses them. No shared mock packages. Examples: `mockLLM`, `mockFileSystem`, `mockChecksumManager`, `mockPathResolver`.

2. **`testify/mock`** — used in some files for convenience (e.g., `mockBus` in history tests). Prefer hand-rolled for new tests.

The `internal/agent/mock_llm_test.go` provides the central mock LLM pattern — it implements `domain.LLM` with configurable streams, error injection, tool calls, and reasoning content. Study this file when adding new agent tests.

### Golden File Tests

Some UI tests use golden files (snapshot testing). Update goldens only when the visual change is intentional:

```bash
go test -update ./internal/ui/prompt/...
```

### Architecture Tests

`internal/architecture_test.go` uses `go-arctest` to enforce package dependency rules. If you introduce a new import that violates the layering, this test will fail.

---

## Adding a New Tool

Tools are the primary extension point in AutoCmd. To add one:

### 1. Create the package

```
internal/tool/<name>/
├── <name>.go          — Tool implementation
├── <name>_test.go     — Tests
```

### 2. Implement the tool interface

Your tool must implement `einotool.BaseTool`:

```go
type Tool struct{}

func (t *Tool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name:        "<name>",
        Description: "What this tool does",
        Parameters:  jsonschema for arguments,
    }, nil
}

func (t *Tool) InvokableRun(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
    // Parse arguments, execute, return result string for LLM consumption
}
```

### 3. Optionally implement extended interfaces

```go
func (t *Tool) Preview(input *compose.ToolInput) domain.ToolDisplay
func (t *Tool) PreflightValidate(input *compose.ToolInput) error
func (t *Tool) IsConcurrentSafe() bool
```

### 4. Register the tool

Add it to the tool list in `cmd/root.go` where the registry is populated:

```go
registry.MustRegister(&mytool.Tool{})
```

### 5. Set permissions

If your tool modifies state, configure it as `ask` mode in the default permission config (`internal/config/defaults.go`):

```go
"by_tool": {
    "<name>": "ask",
}
```

### 6. Test

Write tests covering:
- Normal execution paths
- Input validation failures
- Cancellation via context
- Concurrent safety (if applicable)

---

## Pull Request Process

1. Ensure all tests pass with race detection: `go test -race ./...`
2. Ensure lint passes with zero warnings: `golangci-lint run --timeout=5m`
3. Ensure architecture tests pass: `go test -run TestArchitecture ./internal/`
4. Write a clear PR description explaining what changed and why.
5. CI will verify all of the above automatically on push to the PR branch.
6. Maintain a clean commit history — squash if needed before merge.

### CI Pipeline

The project uses GitHub Actions with two parallel jobs:

| Job | Command |
|-----|---------|
| Test | `go test -race ./...` (Go 1.26.2, ubuntu-latest) |
| Lint | `golangci-lint` v2.12.1 with 33 linters + `gofumpt` + `goimports` |

---

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

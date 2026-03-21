# Testing and TDD

This project follows **strict test-driven development (TDD)** for behavior changes and refactors.

## Workflow: red → green → refactor

1. **Red — failing test first**  
   Add or extend a test that describes the desired behavior. It **must fail** on the current code before you implement anything. The failure proves the test guards the change.

2. **Green — minimal implementation**  
   Write the smallest amount of production code that makes the test pass.

3. **Refactor**  
   Improve structure with all tests still passing.

## Rules of thumb

- **No production change without a preceding failing test**, except:
  - Purely mechanical edits (rename across repo, formatting) when behavior is unchanged.
  - Doc-only or config-comment-only changes.
- If something is hard to test, **narrow the design** (extract pure functions, small interfaces, fake clocks) until you can write the test first.
- Run **`go test ./...`** (or the relevant package) before considering work done.

## Go-specific notes

- Table-driven tests are fine; keep cases independent.
- For Bubble Tea models, prefer driving `Update` with concrete `tea.Msg` values and asserting state / side effects (see `internal/ui/prompt/*_test.go`).
- Golden / snapshot tests: update goldens only with **`go test -update`** when the visual output change is intentional.

## Related docs

- [Architecture](architecture.md) — system boundaries useful when choosing what to test first.
- [Design principles](design.md) — TDD helps enforce good design.

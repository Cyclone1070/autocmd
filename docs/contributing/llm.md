# llm Package

## 1) Purpose

The `llm` package integrates provider backends and resolves model IDs to concrete `domain.LLM` instances.

## 2) Scope

### Owns
- Provider registry and provider resolution.
- Provider adapters (for example Google Gemini).
- Provider/model listing metadata.

### Does NOT own
- Workflow retry/orchestration policy.
- UI rendering behavior.
- Tool execution behavior.

## 3) Public Contract

### Inputs
- Provider/model IDs in `provider/model` format.
- Credentials via configured credential resolution.
- Prompt message history and tool declarations for generation.

### Outputs
- `domain.LLM` instances.
- Token count and streaming results (`eino StreamReader`).
- Wrapped provider/backend errors.

### Invariants
- Invalid IDs/credentials are surfaced as explicit errors.
- Provider failures are not silently swallowed.
- Provider listing order is deterministic (sorted by provider ID).
- Model listing is deterministic by provider-group order (providers sorted by ID; models emitted in each provider's declared order).

## 4) Runtime Behavior

1. Parse provider/model identifier.
2. Resolve provider and credentials.
3. Materialize provider-backed `domain.LLM`.
4. Delegate token counting and streaming to provider client.

Cancellation:
- Context cancellation should be forwarded and surfaced.

Concurrency:
- No long-lived package-level workers; call-scoped operations.

## 5) Error Handling Policy

LLM package should internalize little: most backend boundary failures must be returned.

| Scenario | Internalize? | Return error? | Typical action |
| --- | --- | --- | --- |
| Optional provider metadata unavailable | Yes | No | Omit optional data |
| Invalid model/provider ID | No | Yes | Return validation error |
| Missing/invalid credential | No | Yes | Return explicit credential error |
| Provider API failure | No | Yes | Return wrapped backend error |
| Stream iteration error | No | Yes (`Stream.Err`) | Surface to caller |
| Context cancellation | No | Yes (`ctx.Err`) | Abort promptly |

## 6) Dependencies

### Depends on
- `domain` LLM/provider interfaces.
- Provider SDKs in provider subpackages.

### Must NOT depend on
- Workflow package logic.
- UI rendering package logic.

## 7) Testing Expectations

- Registry tests cover ID parsing, unknown provider/model, and credential resolution paths.
- Provider adapter tests cover stream conversion, token counting, and error propagation.
- Tests should assert wrapped error context for boundary failures.

## 8) Related Docs

- [doc_standard.md](doc_standard.md)
- [architecture.md](architecture.md)
- [workflow.md](workflow.md)
- [testing.md](testing.md)
- [design.md](design.md)

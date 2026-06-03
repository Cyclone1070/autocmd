# AutoCmd

**A terminal-native AI assistant with file system and shell access.** AutoCmd lives in your terminal, understands your working directory, and can read, write, and execute commands on your behalf — all through a rich Bubbletea TUI.

[![CI](https://github.com/Cyclone1070/autocmd/actions/workflows/ci.yml/badge.svg)](https://github.com/Cyclone1070/autocmd/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Cyclone1070/autocmd)](https://goreportcard.com/report/github.com/Cyclone1070/autocmd)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

<img width="1108" height="915" alt="Screenshot 2026-06-03 at 13 05 23" src="https://github.com/user-attachments/assets/feeb5fe2-1eab-4bc0-a7ff-9f22f4d0c6e3" />


## Features

- **AI-powered chat** — Ask questions, run commands, and manipulate files using natural language.
- **Saved commands** — Ask the AI to save a useful command, then run it instantly with `autocmd <name>` — zero AI overhead, zero latency.
- **Bash execution** — The AI can run shell commands in your working directory, with interactive permission prompts for sensitive operations.
- **File system access** — Read, write, and edit files directly from the conversation. Changes are tracked with checksums for safety.
- **Glob & grep search** — Search across your codebase using familiar patterns.
- **Background task management** — Start long-running processes, list active tasks, and stop them on demand.
- **Multi-provider LLM support** — Works with Google Gemini and GitHub Models (Claude, GPT, Gemini). Extensible via MCP tools.
- **Session management** — Conversations are persisted per-directory. Switch, rename, or start fresh sessions.
- **MCP tool support** — Integrate external tools via the [Model Context Protocol](https://modelcontextprotocol.io).
- **Beautiful, Convenient TUI** — Rendered with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Lip Gloss](https://github.com/charmbracelet/lipgloss), and [Glamour](https://github.com/charmbracelet/glamour).
- **Debug logging** — Enable `--debug` to write a detailed log to `~/.config/autocmd/debug.log`.

---

## Prerequisites

- **Go 1.26.2+** — AutoCmd is installed via `go install`. If you don't have Go, download it from [go.dev](https://go.dev/dl/).
- **An LLM provider API key** — Google Gemini (API key) or GitHub Models (token).

---

## Installation

```bash
go install github.com/Cyclone1070/autocmd@latest
```

This downloads, builds, and places the `autocmd` binary in your `$GOPATH/bin` (or `$HOME/go/bin`). Make sure that directory is on your `PATH`:

```bash
# Add to ~/.bashrc, ~/.zshrc, or equivalent
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Verify

```bash
autocmd -v
```

---

## Quick Start

### 1. Authenticate with an LLM provider

```bash
autocmd auth
```

Follow the interactive prompts to set up your API key. AutoCmd supports Google and GitHub providers.

### 2. Select a model

```bash
autocmd model
```

Choose your default LLM model from the available options.

### 3. Run your first prompt

```bash
autocmd list all files in this directory
```

AutoCmd will start a session, process your request using the LLM, and show you the AI's reasoning and tool calls in real time.

### 4. Start a fresh session

```bash
autocmd -n "what's the largest file here?"
```

Or create a new session explicitly:

```bash
autocmd session new
```

### 5. View chat history

```bash
autocmd history
```

---

## Saved Commands

When the AI runs a command you find useful, ask it to save it:

```
You: "save this command as 'status'"
```

Later, run it directly without invoking the AI:

```bash
autocmd status
```

Saved commands execute in a plain shell — no AI loop, no latency, no token cost.

---

## CLI Reference

### Synopsis

```
autocmd [prompt] [flags]
autocmd [command]
```

### Commands

| Command         | Description                                               |
|-----------------|-----------------------------------------------------------|
| `auth`          | Manage authentication for LLM providers                   |
| `completion`    | Generate the autocompletion script for the specified shell |
| `help`          | Help about any command                                    |
| `history`       | View chat history for the current session                 |
| `info`          | Show information about the current configuration and state |
| `model`         | Choose the default LLM model                              |
| `session`       | Manage conversation sessions                              |
| `uninstall`     | Remove AutoCmd and clean up configuration files           |

### Global Flags

| Flag           | Description                                                    |
|----------------|----------------------------------------------------------------|
| `--debug`      | Enable debug logging to `~/.config/autocmd/debug.log`          |
| `-h, --help`   | help for autocmd                                               |
| `-n, --new`    | Start a new session for this prompt                            |

### Prompt Usage

When you pass a text prompt directly, AutoCmd runs it through the AI agent:

```bash
autocmd "explain the architecture of this project"
```

If no prompt is given, the help screen is displayed.

---

## Testing

```bash
go test ./... -race -cover
```

Every implementation file has a companion test file — enforced per project convention. Tests use:

- **Dependency injection unit test** — inject mock implementations for all dependencies for production grade testing
- **Mock-based LLM testing** — deterministic agent behavior without external API calls
- **Table-driven tests** — following idiomatic Go patterns
- **Race detection** — enabled in CI to catch data races early
- **Zero lint warnings** — `golangci-lint` enforced in CI with strict configuration

---

## Configuration

AutoCmd stores its configuration in **`~/.config/autocmd/`**. The default configuration can be customized with a JSON file at that path.

Key configuration areas:

| Section      | Description                                               |
|--------------|-----------------------------------------------------------|
| `tools`      | File size limits, max iterations, per-tool permissions    |
| `providers`  | LLM provider model lists (Google, GitHub)                 |
| `ui`         | Theme colors, chat window width, output window sizes      |

By default, destructive operations (`edit_file`, `write_file`, `bash`) prompt for permission. Read-only tools (`read`, `glob`, `grep`) are allowed automatically.

---

## MCP Tool Support

AutoCmd supports the [Model Context Protocol](https://modelcontextprotocol.io) (MCP) for integrating external tools. Add an `mcp.json` configuration file to `~/.config/autocmd/` to register MCP servers. The AI will automatically discover and use those tools alongside its built-in capabilities.

---

## Session Management

Sessions are automatically scoped to your current Git repository root (or working directory). This means switching projects gives you a fresh context automatically.

| Command                            | Description                            |
|------------------------------------|----------------------------------------|
| `autocmd session`                  | Open the session picker UI             |
| `autocmd session new`              | Create a new chat session              |
| `autocmd -n "your prompt"`         | Run a prompt in a new session          |
| `autocmd history`                  | View conversation history              |

---

## Uninstall

```bash
autocmd uninstall
```

This removes the entire `~/.config/autocmd/` directory, including configuration files, authentication tokens, session data, and saved commands.

To also remove the binary:

```bash
rm "$(which autocmd)"
```

---

## Architecture

Dependencies flow inward: `cmd/` → `internal/*` → `domain/`. The `domain/` package has zero dependencies on the rest of the codebase.

```
cmd/            — Cobra CLI command definitions and dependency wiring
internal/
  agent/        — LLM agent loop, tool scheduling, summarization
  auth/         — Provider authentication (API keys, OAuth)
  command/      — Saved command storage
  config/       — Configuration loading, defaults, validation
  domain/       — Shared types, constants, events
  eventbus/     — In-process event bus for UI updates
  fs/           — File system abstraction
  logging/      — Structured logging
  permission/   — Per-tool permission resolution
  provider/     — LLM provider registry (Google, GitHub)
  session/      — Session persistence and lookup
  state/        — Application state management
  tool/         — Tool implementations (bash, read, write, edit, glob, grep, save, MCP)
  ui/           — Bubbletea UI models and renderers
  workflow/     — Orchestration logic (prompt, auth, model picker, etc.)
```

Key design decisions:

- **Graph-runner state machine** — agent orchestration uses composable graph nodes, not a monolithic loop. Each turn is an event-driven cycle through the graph.
- **Event-driven UI** — the agent never imports TUI code. It emits typed events over an in-process bus; the Bubbletea renderer subscribes independently.
- **Tool abstraction** — all tools implement a uniform interface with metadata, input schemas, and permission levels. The agent discovers available tools at runtime.

---

## Built With

- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [Bubbletea](https://github.com/charmbracelet/bubbletea) — Terminal UI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Style definitions
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering
- [Eino](https://github.com/cloudwego/eino) — LLM orchestration framework
- [MCP Go](https://github.com/mark3labs/mcp-go) — Model Context Protocol client
- [golangci-lint](https://golangci-lint.run) — Strict lint enforcement, zero-warning policy
- [GitHub Actions](https://github.com/features/actions) — CI pipeline (test + lint on every push)

---

## License

[MIT](LICENSE) — Copyright (c) 2026 Cyclone1070

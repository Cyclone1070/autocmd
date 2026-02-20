# UI Behavior & Architecture

The application is designed to behave like a standard, well-behaved CLI tool rather than a full-screen TUI application.

## Core Interaction Model

1.  **Standard Output Flow**: The app writes directly to stdout. It does **not** use the "alternate screen" buffer (the mode that clears the screen and takes over, used by tools like `vim` or `less`).
2.  **Terminal History**: We respect the user's scrollback. Previous turns in the conversation are committed to the terminal history and are managed by the terminal emulator, not the application.

## Rendering Pipeline

The UI is split into two distinct components:

### 1. Flushed History (Static)
This is content that has been finalized. Once a piece of content is determined to be "complete," it is printed to stdout and effectively forgotten by the UI rendering engine. This ensures O(1) memory usage relative to the conversation length for the active renderer.

### 2. Dynamic Pending View (Live)
This is the active area at the bottom of the stream. It uses Bubble Tea to handle the dynamic updates of the currently streaming response. This view is transient; as parts of it become stable, they are moved to Flushed History.

## The "Safe Block" Flushing Strategy

To render Markdown correctly while streaming, we cannot simply flash every character, as Markdown elements (like code blocks, tables, or multi-line blockquotes) depend on context.

We use a **Block-Based Flushing** strategy:

1.  **Accumulation**: Incoming text is accumulated in the Pending View buffer.
2.  **Block Detection**: The buffer is parsed (using Goldmark or similar) to identify discrete Markdown blocks (paragraphs, headers, code blocks).
3.  **Safety Heuristic**:
    *   **Safe Blocks**: All blocks *except the very last one* are considered "Safe". They are fully formed and unlikely to change structure.
    *   **Unsafe Block**: The **very last block** is considered "Unsafe". It is potentially incomplete (e.g., an open code fence ` ```go ` or a half-written sentence).
4.  **The Flush**:
    *   **Safe Blocks** are rendered to ANSI strings and printed to stdout immediately. They are then removed from the accumulator.
    *   The **Unsafe Block** remains in the accumulator and is rendered in the Dynamic Pending View.

### Example

Stream: `Here is a list:\n\n1. Item one\n2. Ite`

*   **Block 1 (`Here is a list:`)**: Followed by a newline. **SAFE**. -> Flush to generic stdout.
*   **Block 2 (`1. Item one`)**: Followed by a newline and another item. **SAFE**. -> Flush to generic stdout.
*   **Block 3 (`2. Ite`)**: The last block. **UNSAFE**. -> Render in Bubble Tea view (likely showing a spinner or simply the incomplete text).

This ensures that we never "flash" or "jitter" properly formatted history, while retaining the responsiveness of a live-updating CLI.

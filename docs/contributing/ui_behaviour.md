# UI Architecture & Behavior Guideline

This document provides the definitive source of truth for the `internal/ui` package's architecture, rendering stack, and flushing behavior.

**Implementation note:** The UI is implemented via `internal/ui/engine` (pure state/transitions), `internal/ui/tea` (Bubble Tea adapter), `internal/ui/markdown` (streaming markdown), `internal/ui/compose` (entrypoint wiring + engine DI), `internal/ui/theme` (styling), `internal/ui/tool` (tool output display), `internal/ui/layout` (viewport truncation), and `internal/ui/cursor` (terminal cursor detection). Entrypoints use `compose.NewRenderer` exclusively. No legacy model/update/view path remains.

## 1. High-Level Concept: The "Split View" Model

The application UI is fundamentally a **Split View** system composed of two distinct areas:

1.  **History (The Past):** Content that has been finalized, printed to `stdout`, and is now managed by the terminal emulator's scrollback buffer. The application *cannot* modify this once written.
2.  **View (The Present):** The active, dynamic rendering area at the bottom of the screen. This is managed by Bubble Tea (`View()` method) and is redrawn on every frame. It contains pending markdown, active tools, padding, and the status bar.

These two areas meet at the **Flush Line**. As content stabilizes, it crosses the line from View to History.

---

## 2. Layering & Ordering (Bottom-Up)

The UI is constructed from the bottom up. The `View()` method renders the following stack:

| Layer                  | Component         | Description                                                             | Management         |
| :--------------------- | :---------------- | :---------------------------------------------------------------------- | :----------------- |
| **4. History**         | `stdout`          | Old content. Invisible to `View()`. Visible in terminal scrollback.     | Terminal           |
| **3. Pending Content** | `engine.Render`   | Unfinished/Unsafe markdown blocks + Active Code Blocks + Running Tools. | `internal/ui/markdown` |
| **2. Padding**         | `engine.Render`   | Whitespace (`\n` * N) used to pin the Status Bar to the bottom.         | `engine` |
| **1. Status Bar**      | `engine.Render`   | The bottom anchor (`\n\n` + Spinner + text). Always present.            | `engine` / `tea` |

**Visual Stack:**
```text
[HISTORY (Terminal Scrollback)]
----------------(Flush Line)----------------
[PENDING CONTENT (Markdown / Tools)]  <-- Grows downwards
[PADDING (Dynamic Whitespace)]        <-- Shrinks as content grows
[STATUS BAR (Pinned to Bottom)]       <-- Fixed position
```

---

## 3. Component Details

### A. Pending View (`internal/ui/markdown`)
*   **Purpose:** buffers partial markdown and incomplete blocks (e.g., an open code fence `` ```go ``).
*   **Behavior:**
    *   Appends new tokens as they arrive.
    *   Uses `Goldmark` to parse the buffer.
    *   Identifies "Safe Blocks" (fully closed paragraphs, completed lists) vs "Unsafe Blocks" (the last growing element).
    *   **Only** "Safe Blocks" are eligible for flushing.

> [!IMPORTANT]
> **The "Always One Unsafe Block" Invariant**
>
> During streaming, the View **always** contains at least the last top-level markdown block as a live preview. The markdown stream uses a "hold one back" design: it only flushes a block when a **newer** block arrives to confirm the previous one is complete. The last block is never flushed by `Append()` because it might still be growing.
>
> This means that after every flush, the View is never empty — it contains the next paragraph's live preview (`Pending()`) plus the status bar.
>
> **Exception — Done / Cancel:** When the session ends (`DoneEvent` or `Ctrl+C`), `RenderRemaining()` is called, which flushes **everything** in the buffer including the last unsafe block. After this final flush the View is empty and the engine quits.

### B. Padding Area (`maxContentHeight`)
*   **Purpose:** Ensures the Status Bar remains visually pinned to the bottom of the viewport, even when the pending content is small.
*   **Calculation:** `Padding = maxContentHeight - currentContentHeight`.
*   **Behavior:**
    *   `maxContentHeight` starts at `TerminalHeight - CursorRow`.
    *   As `currentContentHeight` grows, Padding shrinks.
    *   When `currentContentHeight > maxContentHeight`, Padding becomes 0.

### C. Status Bar
*   **Purpose:** Indicates application state (Thinking, Generating, Done).
*   **Behavior:** Always appended at the very end of `View()`. Prefixed with `\n\n` to ensure separation.

---

## 4. Flushing: The Transition from View to History

**"Flushing"** is the process of moving content from the dynamic `View` to the static `History`.

*   **Direction:** Top-Down (Oldest content flushes first).
*   **Trigger:** When `internal/ui/markdown` detects ≥2 top-level markdown blocks. The second-to-last block is considered "Safe" and is flushed. The last block is **kept** as a live preview (see the "Always One Unsafe Block" invariant above).
*   **Mechanism:**
    1.  **Extract:** The safe block is removed from the `streaming` buffer.
    2.  **Print:** The block is sent to `stdout` via `tea.Println` (or `tea.Printf`).
    3.  **Resize:** The `maxContentHeight` is **decremented** by the height of the flushed block. This is critical to keep the View stable.
    4.  **After flush:** The View still contains the last unsafe block (via `Pending()`) plus the status bar. It is **never** empty during streaming.

**Why Flush?**
*   To prevent the `View` from growing infinitely (which causes flickering and memory issues).
*   To "commit" content so it can't be changed (simulating a stream).

---

## 5. Visual Guide: The Code Block Flush

Here is the exact frame-by-frame behavior of flushing 8 lines where the flush threshold is 3 lines.

**Scenario:**
*   Terminal Height: 10 lines.
*   Status Bar: 2 lines (Implied Blank Line + Text Line).
*   Content Area: 8 lines (Row 1-8).
*   Content starts on Row 2 (leaving Row 1 initially empty).
*   User streams 8 lines.
*   **Flush Threshold:** 3 lines (Lines 1-3 flush, then Lines 4-6 flush, then Lines 7-8 remain pending).
*   **Note:** After each flush the last streamed line stays in the View as the unsafe `Pending()` block.

### Frame 1: Stream Line 1
*Content starts at Row 2.*
```text
+------------------------------+ Line 1
|                              |
| Line 1 (Pending)             | <--- View starts here
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+ Line 10
```

### Frame 2: Stream Line 2
*Content grows down. Line 1 stays put.*
```text
+------------------------------+
|                              |
| Line 1 (Pending)             |
| Line 2 (Pending)             | <--- New Content
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 3: Stream Line 3
*Content grows down.*
```text
+------------------------------+
|                              |
| Line 1 (Pending)             |
| Line 2 (Pending)             |
| Line 3 (Pending)             | <--- New Content
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 4: FLUSH TRIGGERED (Lines 1-3 Safe)
*Lines 1-3 move to History (Filling Rows 2, 3, 4).*
*`maxContentHeight` drops. Line 4 was already buffered as the unsafe block, so it stays in the View as `Pending()`. NO VISUAL MOVEMENT.*
```text
+------------------------------+
|                              |
| Line 1 (History)             | <--- Flushed! (Stayed put)
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)             | <--- Unsafe block kept in View
|                              | <--- Padding matches new height
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 5: Stream Line 5
*New content grows the pending block (Row 6).*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)             |
| Line 5 (Pending)             | <--- New Content (Row 6)
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 6: Stream Line 6
*Content reaches Row 7.*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)             |
| Line 5 (Pending)             |
| Line 6 (Pending)             | <--- New Content (Row 7)
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 7: FLUSH TRIGGERED (Lines 4-6 Safe)
*Lines 4-6 move to History (Filling Rows 5, 6, 7).*
*Line 7 was already buffered as the unsafe block, so it stays in the View as `Pending()`. NO VISUAL MOVEMENT.*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (History)             | <--- Flushed! (Stayed put)
| Line 5 (History)             |
| Line 6 (History)             |
| Line 7 (Pending)             | <--- Unsafe block kept in View
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 8: Stream Line 8 (SCROLLING!)
*Content needs Row 9. Row 9 is Status Bar. Pushes everything UP 1 line.*
*Row 1 (Empty) moves off-screen. Line 1 moves to Row 1.*
```text
+------------------------------+
| Line 1 (History)             | <--- Was Row 2. Now Row 1.
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (History)             |
| Line 5 (History)             |
| Line 6 (History)             |
| Line 7 (Pending)             |
| Line 8 (Pending)             | <--- New Content (Row 8)
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

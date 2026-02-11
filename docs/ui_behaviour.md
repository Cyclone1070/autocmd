# UI Architecture & Behavior Guideline

This document provides the definitive source of truth for the `internal/ui` package's architecture, rendering stack, and flushing behavior.

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
| **3. Pending Content** | `renderContent()` | Unfinished/Unsafe markdown blocks + Active Code Blocks + Running Tools. | `streaming.go`     |
| **2. Padding**         | `renderView()`    | Whitespace (`\n` * N) used to pin the Status Bar to the bottom.         | `maxContentHeight` |
| **1. Status Bar**      | `statusBar()`     | The bottom anchor (`\n\n` + Spinner + text). Always present.            | `model.go`         |

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

### A. Pending View (`streaming.go`)
*   **Purpose:** buffers partial markdown and incomplete blocks (e.g., an open code fence `` ```go ``).
*   **Behavior:**
    *   Appends new tokens as they arrive.
    *   Uses `Goldmark` to parse the buffer.
    *   Identifies "Safe Blocks" (fully closed paragraphs, completed lists) vs "Unsafe Blocks" (the last growing element).
    *   **Only** "Safe Blocks" are eligible for flushing.

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
*   **Trigger:** When `streaming.go` detects ≥2 top-level markdown blocks. The second-to-last block is considered "Safe" and is flushed.
*   **Mechanism:**
    1.  **Extract:** The safe block is removed from the `streaming` buffer.
    2.  **Print:** The block is sent to `stdout` via `tea.Println` (or `tea.Printf`).
    3.  **Resize:** The `maxContentHeight` is **decremented** by the height of the flushed block. This is critical to keep the View stable.

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
*View clears. `maxContentHeight` drops. NO VISUAL MOVEMENT.*
```text
+------------------------------+
|                              |
| Line 1 (History)             | <--- Flushed! (Stayed put)
| Line 2 (History)             |
| Line 3 (History)             |
|                              | <--- View is momentarily empty (Row 5)
|                              | <--- Padding matches new height
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 5: Stream Line 4
*New content starts filling the cleared View (Row 5).*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)             | <--- New View Content (Row 5)
|                              | <--- Padding
|                              | <--- Padding
|                              | <--- Padding
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 6: Stream Line 5
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

### Frame 7: Stream Line 6
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

### Frame 8: FLUSH TRIGGERED (Lines 4-6 Safe)
*Lines 4-6 move to History (Filling Rows 5, 6, 7).*
*View clears. `maxContentHeight` drops. NO VISUAL MOVEMENT.*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (History)             | <--- Flushed! (Stayed put)
| Line 5 (History)             |
| Line 6 (History)             |
|                              | <--- View is momentarily empty (Row 8)
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 9: Stream Line 7
*New content fills remaining View space (Row 8).*
```text
+------------------------------+
|                              |
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (History)             |
| Line 5 (History)             |
| Line 6 (History)             |
| Line 7 (Pending)             | <--- New View Content (Row 8)
| [STATUS BAR]                 | <--- Status Bar (2 lines)
+------------------------------+
```

### Frame 10: Stream Line 8 (SCROLLING!)
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

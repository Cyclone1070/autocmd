# UI Architecture & Behavior Guideline

This document provides the definitive source of truth for the `internal/ui` package's architecture, rendering stack, and flushing behavior.

**Implementation note:** The UI is implemented via `internal/ui/engine` (pure state/transitions), `internal/ui/tea` (Bubble Tea adapter), `internal/ui/markdown` (streaming markdown), `internal/ui/compose` (entrypoint wiring + engine DI), `internal/ui/theme` (styling), `internal/ui/tool` (tool output display), `internal/ui/layout` (viewport truncation), and `internal/ui/cursor` (terminal cursor detection). Entrypoints use `compose.NewRenderer` exclusively. No legacy model/update/view path remains.

---

## 1. High-Level Concept: Natural Flow

The UI outputs content as a **natural terminal stream**. There is no pinned status bar, no padding, and no viewport anchoring during a session. Content flows downward exactly like a standard CLI tool.

There are two areas:

1.  **History (The Past):** Content that has been finalized, printed to `stdout` via `tea.Println`, and is now managed by the terminal emulator's scrollback buffer. The application *cannot* modify this once written.
2.  **View (The Present):** The active, dynamic rendering area at the bottom of the screen. This is managed by Bubble Tea (`View()` method) and is redrawn on every frame. It contains only the pending markdown block, active tools, and optionally the animated activity indicator.

These two areas meet at the **Flush Line**. As content stabilizes, it crosses the line from View to History.

---

## 2. View Composition (Bottom-Up)

The `View()` method renders the following stack. There is **no padding layer**.

| Layer                     | Component       | Description                                                         | Management             |
| :------------------------ | :-------------- | :------------------------------------------------------------------ | :--------------------- |
| **3. History**            | `stdout`        | Old content. Invisible to `View()`. Visible in terminal scrollback. | Terminal               |
| **2. Pending Content**    | `engine.Render` | Last unsafe markdown block + Active/Queued Tools.                   | `internal/ui/markdown` |
| **1. Activity Indicator** | `engine.Render` | Animated `...` appended **inline** to content when idle.            | `engine`               |

**Visual Stack:**
```text
[HISTORY (Terminal Scrollback)]
----------------(Flush Line)----------------
[PENDING CONTENT (Last Unsafe Block / Tools) ... (Activity Indicator)]
```

**On session end**, a static status bar is printed as the final line of History:

```text
[HISTORY (Terminal Scrollback)]
[✓ Done     Context: 42%]   ← or [✗ Cancelled     Context: 42%]
```

---

## 3. Component Details

### A. Pending View (`internal/ui/markdown`)
*   **Purpose:** Buffers partial markdown and incomplete blocks (e.g., an open code fence `` ```go ``).
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
> This means that after every flush, the View is never empty — it contains the next paragraph's live preview (`Pending()`) plus any active tools.
>
> **Exception — Done / Cancel:** When the session ends (`DoneEvent` or `Ctrl+C`), `RenderRemaining()` is called, which flushes **everything** in the buffer including the last unsafe block. After this final flush the View is empty and the engine quits.

### B. Activity Indicator (`...`)
*   **Purpose:** Tells the user the session is still alive when no visible output is being produced.
*   **When shown:** The indicator is appended **inline** to the end of the last line of the View when ALL of the following are true:
    *   Session is running (no `DoneEvent` or `Ctrl+C` received).
    *   No text is actively being typed (simulated typing buffer is empty).
    *   No tools are actively running.
*   **When hidden:** The indicator disappears immediately when new text arrives or a tool starts.
*   **Animation:** Three dots animating (e.g., `.` → `..` → `...` → `.` cycling). Driven by Bubble Tea's tick mechanism.

### C. Simulated Typing
*   **Purpose:** All `TextEvent` content is rendered with a simulated typing effect built into the UI layer. Characters appear incrementally rather than in bulk chunks.
*   **Behavior:**
    *   When a `TextEvent` arrives, its text is added to an internal typing buffer.
    *   On each tick, a batch of characters (e.g., 2-4 characters) is moved from the buffer to the markdown stream (`Append`).
    *   The flush logic runs normally on each append — safe blocks are flushed to history as they complete.
    *   When the typing buffer is empty and no tools are running, the Activity Indicator (`...`) appears.

### D. Status Bar (Exit Only)
*   **Purpose:** Indicates final application state.
*   **Behavior:** NOT shown during the session. Printed as a single `tea.Println` when the session ends (after all content and tools are flushed).
*   **Format:** Static text, no spinner. Example: `✓ Done     Context: 42%` or `✗ Cancelled     Context: 42%`.

---

## 4. Flushing: The Transition from View to History

**"Flushing"** is the process of moving content from the dynamic `View` to the static `History`.

*   **Direction:** Top-Down (Oldest content flushes first).
*   **Trigger:** When `internal/ui/markdown` detects ≥2 top-level markdown blocks. The second-to-last block is considered "Safe" and is flushed. The last block is **kept** as a live preview (see the "Always One Unsafe Block" invariant above).
*   **Mechanism:**
    1.  **Extract:** The safe block is removed from the `streaming` buffer.
    2.  **Print:** The block is sent to `stdout` via `tea.Println` (or `tea.Printf`).
    3.  **After flush:** The View still contains the last unsafe block (via `Pending()`) plus any active tools. It is **never** empty during streaming.

**Why Flush?**
*   To prevent the `View` from growing infinitely (which causes flickering and memory issues).
*   To "commit" content so it can't be changed (simulating a stream).

---

## 5. Event Flow

The engine processes the following events (in order of a typical session):

### Session Lifecycle
```
TextEvent → TextEvent → ... → ToolStartEvent → ToolStreamEvent → ToolEndEvent → ... → DoneEvent
```

### Event Details

| Event             | Engine Action                                                                                  |
| :---------------- | :--------------------------------------------------------------------------------------------- |
| `TextEvent`       | Add text to simulated typing buffer. Typing effect drains buffer over subsequent ticks.        |
| `ToolStartEvent`  | Flush remaining markdown. Create a new `ToolState` (status: running). Hide activity indicator. |
| `ToolStreamEvent` | Append chunk to the tool's shell output.                                                       |
| `ToolEndEvent`    | Mark tool as success/error. Flush completed leading tools to history.                          |
| `DoneEvent`       | Flush all remaining markdown and tools. Print static status bar. Quit.                         |
| `Ctrl+C`          | Same as `DoneEvent` but status bar shows "Cancelled".                                          |

### Tick-Driven Behavior

On each Bubble Tea tick:
1.  If the typing buffer has content: drain a few characters → `Markdown.Append()` → possibly flush.
2.  If the typing buffer is empty and no tools are running and session is alive: show animated `...`.
3.  Update the `...` animation frame.

---

## 6. Visual Guide: Natural Flow Flush

**Scenario:**
*   Terminal Height: 10 lines.
*   User streams 4 lines of text. Lines 1-3 form a complete paragraph, Line 4 starts a new one.
*   Flush threshold: When new block starts, old block is flushed.

### Frame 1-3: Streaming Lines 1-3
*Content appears character by character via simulated typing.*
```text
| Line 1 (Pending)             |
| Line 2 (Pending)             |
| Line 3 (Pending)             |
```

### Frame 4: Line 4 starts → Flush Triggered
*Lines 1-3 form a complete paragraph. Line 4 starts a new paragraph → Lines 1-3 are safe → Flushed.*

**After Println + View re-render:**
```text
| Line 1 (History)             |  ← Flushed via Println (stayed in place)
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)             |  ← View starts here (last unsafe block)
```

Lines 1-3 physically stayed in the same terminal rows. They just transitioned from "managed by View" to "managed by terminal scrollback." The View now only contains Line 4.

### Frame 5: Session idle
*No text arriving, no tools running, session still alive.*
```text
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (Pending)...          |  ← Activity indicator (inline)
```

### Frame 6: DoneEvent
*Everything flushed. Static status bar printed.*
```text
| Line 1 (History)             |
| Line 2 (History)             |
| Line 3 (History)             |
| Line 4 (History)             |  ← Final flush
| ✓ Done     Context: 42%     |  ← Static status bar (History)
```

---

## 7. Key Design Decisions

### No Padding, No Pinning
The previous design pinned a status bar to the bottom of the viewport using dynamic padding. This created an unsolvable coupling between `TotalFlushedLines` and `viewHeight` through `MaxAbsoluteHeight`, leading to circular bugs (flash vs. permanent gap). The new design eliminates this entirely.

### No Thinking Event
The previous `ThinkingEvent` started a spinner. The new design replaces this with the `...` activity indicator, which activates automatically when the UI is idle. No explicit "thinking" state is needed.

### Simulated Typing
Typing simulation is a UI concern, not a domain concern. The engine's typing buffer smooths out bursty `TextEvent` delivery, creating a natural typing feel and ensuring flushes happen at natural breakpoints (as characters arrive one by one, block boundaries are detected cleanly).

### Status Bar at Exit Only
The status bar is a summary, not a live indicator. Showing it only at exit avoids all the layout complexity of maintaining a persistent UI element during streaming.
